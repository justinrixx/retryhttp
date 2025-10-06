package retryhttp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// DefaultMaxRetries is the default maximum retries setting. This can be configured using
// [WithMaxRetries].
const DefaultMaxRetries = 3

var (
	// ErrBufferingBody is a sentinel that signals an error before the response was sent. Since
	// request body streams can only be consumed once, they must be buffered into memory before
	// the first attempt. If an error occurs during that buffering process, it is returned
	// in a new error wrapping this sentinel. A caller can identify this case using
	// errors.Is(err, ErrBufferingBody).
	ErrBufferingBody = errors.New("error buffering body before first attempt")

	// ErrSeekingBody is a sentinel that signals an error preparing for a new attempt by
	// rewinding the stream back to the beginning. If an error occurs during that seek, it is
	// returned in a new error wrapping this sentinel. A caller can identify this case using
	// errors.Is(err, ErrSeekingBody).
	ErrSeekingBody = errors.New("error seeking body buffer back to beginning after attempt")

	// ErrRetriesExhausted is a sentinel that signals all retry attempts have been exhausted.
	// This error will be joined with the last attempt's error to provide clearer context
	// about why the request failed. A caller can identify this case using
	// errors.Is(err, ErrRetriesExhausted).
	ErrRetriesExhausted = errors.New("max retries exhausted")
)

type (
	// Attempt is a collection of information used by [ShouldRetryFn] and [DelayFn] to determine
	// if a retry is appropriate and if so how long to delay.
	Attempt struct {
		// Count represents how many attempts have been made. This includes the initial attempt.
		Count int

		// Req is the HTTP request used to make the request.
		Req *http.Request

		// Res is the HTTP response returned by the attempt. This may be nil if a non-nil error
		// occurred. Note that since the response body is a stream, if you need to inspect it
		// you are responsible for buffering it into memory and resetting the stream to be
		// returned out of the HTTP round trip.
		Res *http.Response

		// Err is an optional error that may have occurred during the HTTP round trip.
		Err error
	}

	// ShouldRetryFn is a callback type consulted by [Transport] to determine if another attempt
	// should be made after the current one.
	ShouldRetryFn func(attempt Attempt) bool

	// DelayFn is a callback type consulted by [Transport] to determine how long to wait before
	// the next attempt.
	DelayFn func(attempt Attempt) time.Duration

	// Transport implements [http.RoundTripper] and can be configured with many options. See
	// the documentation for the [New] function.
	Transport struct {
		rt                   http.RoundTripper
		maxRetries           *int // pointer to differentiate between 0 and unset
		shouldRetryFn        ShouldRetryFn
		delayFn              DelayFn
		preventRetryWithBody bool
		attemptTimeout       time.Duration
		initOnce             sync.Once
	}
)

// New is used to construct a new [Transport], configured with any desired options.
// These options include [WithTransport], [WithMaxRetries], [WithShouldRetryFn],
// [WithDelayFn], and [WithPreventRetryWithBody]. Any number of options may be provided.
// If the same option is provided multiple times, the latest one takes precedence.
func New(options ...func(*Transport)) *Transport {
	tr := &Transport{}

	for _, option := range options {
		option(tr)
	}

	return tr
}

func (t *Transport) init() {
	if t.rt == nil {
		t.rt = http.DefaultTransport
	}
	if t.shouldRetryFn == nil {
		t.shouldRetryFn = DefaultShouldRetryFn
	}
	if t.delayFn == nil {
		t.delayFn = DefaultDelayFn
	}

	if t.maxRetries == nil {
		tmp := DefaultMaxRetries
		t.maxRetries = &tmp
	}
}

// RoundTrip performs the actual HTTP round trip for a request. It performs setup
// and retries, but delegates the actual HTTP round trip to [Transport]'s internal
// roundtripper. This is not intended to be called directly, but rather implements
// the [http.RoundTripper] interface so that it can be passed to a [http.Client] as
// its internal Transport.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.initOnce.Do(t.init)

	var attemptCount int
	ctx := req.Context()

	maxRetries := *t.maxRetries
	ctxRetries, set := getMaxRetriesFromContext(ctx)
	if set {
		maxRetries = ctxRetries
	}

	shouldRetryFn := t.shouldRetryFn
	ctxShouldRetryFn, set := getShouldRetryFnFromContext(ctx)
	if set {
		shouldRetryFn = ctxShouldRetryFn
	}

	delayFn := t.delayFn
	ctxDelayFn, set := getDelayFnFromContext(ctx)
	if set {
		delayFn = ctxDelayFn
	}

	preventRetryWithBody := t.preventRetryWithBody
	ctxPreventRetry, set := getPreventRetryWithBodyFromContext(ctx)
	if set {
		preventRetryWithBody = ctxPreventRetry
	}

	preventRetry := req.Body != nil && req.Body != http.NoBody && preventRetryWithBody

	// if body is present, it must be buffered if there is any chance of a retry
	// since it can only be consumed once.
	var br *bytes.Reader
	if req.Body != nil && req.Body != http.NoBody && !preventRetry {
		var buf bytes.Buffer
		if _, err := io.Copy(&buf, req.Body); err != nil {
			req.Body.Close()
			return nil, fmt.Errorf("%w: %s", ErrBufferingBody, err)
		}
		req.Body.Close()

		br = bytes.NewReader(buf.Bytes())
		req.Body = io.NopCloser(br)
	}

	attemptTimeout := t.attemptTimeout
	ctxAttemptTimeout, set := getAttemptTimeoutFromContext(ctx)
	if set {
		attemptTimeout = ctxAttemptTimeout
	}

	for {
		// set per-attempt timeout if needed
		var cancel context.CancelFunc = func() {}
		reqWithTimeout := req
		if attemptTimeout != 0 {
			var timeoutCtx context.Context
			timeoutCtx, cancel = context.WithTimeout(ctx, attemptTimeout)
			reqWithTimeout = req.WithContext(timeoutCtx)
		}

		// the actual round trip
		res, err := t.rt.RoundTrip(reqWithTimeout)
		attemptCount++

		if preventRetry {
			return injectCancelReader(res, cancel), err
		}

		if attemptCount-1 >= maxRetries {
			if err != nil {
				err = errors.Join(ErrRetriesExhausted, err)
			}
			return injectCancelReader(res, cancel), err
		}

		attempt := Attempt{
			Count: attemptCount,
			Req:   req,
			Res:   res,
			Err:   err,
		}

		shouldRetry := shouldRetryFn(attempt)
		if !shouldRetry {
			return injectCancelReader(res, cancel), err
		}

		delay := delayFn(attempt)
		if br != nil {
			if _, serr := br.Seek(0, 0); serr != nil {
				return injectCancelReader(res, cancel), fmt.Errorf("%w: %s", ErrSeekingBody, err)
			}
			reqWithTimeout.Body = io.NopCloser(br)
		}

		if res != nil {
			_, _ = io.Copy(io.Discard, res.Body)
			res.Body.Close()
		}

		// going for another attempt, cancel the context of the attempt that was just made
		cancel()

		select {
		case <-time.After(delay):
			// do nothing, just loop again
		case <-req.Context().Done(): // happens if the parent context expires
			return nil, req.Context().Err()
		}
	}
}
