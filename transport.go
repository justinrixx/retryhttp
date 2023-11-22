package retryhttp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"time"
)

// DefaultMaxRetries is the default maximum retries setting. This can be configured using
// WithMaxRetries.
const DefaultMaxRetries = 3

var (
	// ErrBufferingBody is a sentinel that signals an error before the response was sent. Since
	// request body streams can only be consumed once, the must be buffered into memory before
	// the first attempt. If an error occurs during that buffering process, it is returned
	// in an errors.Join with this sentinel. A caller can identify this case using
	// errors.Is(err, ErrBufferingBody).
	ErrBufferingBody = errors.New("error buffering body before first attempt")

	// ErrSeekingBody is a sentinel that signals an error preparing for a new attempt by
	// rewinding the stream back to the beginning. If an error occurs during that seek, it is
	// returned in an errors.Join with this sentinel. A caller can identify this case using
	// errors.Is(err, ErrSeekingBody).
	ErrSeekingBody = errors.New("error seeking body buffer back to beginning after attempt")
)

type (
	// Attempt is a collection of information used by ShouldRetryFn and DelayFn to determine
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

	// ShouldRetryFn is a callback type consulted by Transport to determine if another attempt
	// should be made after the current one.
	ShouldRetryFn func(attempt Attempt) bool

	// DelayFn is a callback type consulted by Transport to determine how long to wait before
	// the next attempt.
	DelayFn func(attempt Attempt) time.Duration

	// Transport implements http.RoundTripper and can be configured with many options. See
	// the documentation for the New function.
	Transport struct {
		rt                   http.RoundTripper
		maxRetries           *int // pointer to differentiate between 0 and unset
		shouldRetryFn        ShouldRetryFn
		delayFn              DelayFn
		preventRetryWithBody bool
		attemptTimeout       time.Duration
	}
)

// WithTransport configures a Transport with an internal roundtripper of its own.
// This is often http.DefaultTransport, but it could be anything else.
func WithTransport(transport http.RoundTripper) func(*Transport) {
	return func(t *Transport) {
		t.rt = transport
	}
}

// WithMaxRetries configures the maximum number of retries a Transport is allowed to make.
// If not set, defaults to DefaultMaxRetries. Note that this number does not include the
// initial attempt, so if this is configured as 3, there could be up to 4 total attempts.
func WithMaxRetries(maxRetries int) func(*Transport) {
	return func(t *Transport) {
		t.maxRetries = &maxRetries
	}
}

// WithShouldRetryFn configures the ShouldRetryFn callback to use.
func WithShouldRetryFn(shouldRetryFn ShouldRetryFn) func(*Transport) {
	return func(t *Transport) {
		t.shouldRetryFn = shouldRetryFn
	}
}

// WithDelayFn configures the DelayFn callback to use.
func WithDelayFn(delayFn DelayFn) func(*Transport) {
	return func(t *Transport) {
		t.delayFn = delayFn
	}
}

// WithPreventRetryWithBody configures whether to prevent retries on requests that
// have bodies. This may be desirable because any request that has a chance of
// requiring a retry must have its body buffered into memory by Transport in case
// it needs to be replayed on subsequent attempts. It is up to package consumers
// to determine if and when this behavior is appropriate.
func WithPreventRetryWithBody(preventRetryWithBody bool) func(*Transport) {
	return func(t *Transport) {
		t.preventRetryWithBody = preventRetryWithBody
	}
}

// WithAttemptTimeout configures a per-attempt timeout to be used in requests. A
// per-attempt timeout differs from an overall timeout in that it applies to and is
// reset in each individual attempt rather than all attempts and delays combined.
// If using an overall timeout along with a per-attempt timeout, the stricter of
// the two takes precedence.
func WithAttemptTimeout(attemptTimeout time.Duration) func(*Transport) {
	return func(t *Transport) {
		t.attemptTimeout = attemptTimeout
	}
}

// New is used to construct a new Transport, configured with any desired options.
// These options include WithTransport, WithMaxRetries, WithShouldRetryFn,
// WithDelayFn, and WithPreventRetryWithBody. Any number of options may be provided.
// If the same option is provided multiple times, the latest one takes precedence.
func New(options ...func(*Transport)) *Transport {
	tr := &Transport{}

	for _, option := range options {
		option(tr)
	}

	return tr
}

// RoundTrip performs the actual HTTP round trip for a request. It performs setup
// and retries, but delegates the actual HTTP round trip to Transport's internal
// roundtripper. This is not intended to be called directly, but rather implement
// the http.RoundTripper interface so that it can be passed to a http.Client as
// its internal Transport.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	var attemptCount int
	maxRetries := DefaultMaxRetries
	if t.maxRetries != nil {
		maxRetries = *t.maxRetries
	}

	ctx := req.Context()

	preventRetry := req.Body != nil && req.Body != http.NoBody && t.preventRetryWithBody

	// if body is present, it must be buffered if there is any chance of a retry
	// since it can only be consumed once.
	var br *bytes.Reader
	if req.Body != nil && req.Body != http.NoBody && !preventRetry {
		var buf bytes.Buffer
		if _, err := io.Copy(&buf, req.Body); err != nil {
			req.Body.Close()
			return nil, errors.Join(err, ErrBufferingBody)
		}
		req.Body.Close()

		br := bytes.NewReader(buf.Bytes())
		req.Body = io.NopCloser(br)
	}

	for {
		// set per-attempt timeout if needed
		var cancel context.CancelFunc = func() {}
		reqWithTimeout := req
		if t.attemptTimeout != 0 {
			var timeoutCtx context.Context
			timeoutCtx, cancel = context.WithTimeout(ctx, t.attemptTimeout)
			reqWithTimeout = req.WithContext(timeoutCtx)
		}

		// the actual round trip
		res, err := t.rt.RoundTrip(reqWithTimeout)
		attemptCount++

		if preventRetry || attemptCount-1 >= maxRetries {
			return injectCancelReader(res, cancel), err
		}

		attempt := Attempt{
			Count: attemptCount,
			Req:   req,
			Res:   res,
			Err:   err,
		}

		shouldRetry := t.shouldRetryFn(attempt)
		if !shouldRetry {
			return injectCancelReader(res, cancel), err
		}

		delay := t.delayFn(attempt)
		if br != nil {
			if _, serr := br.Seek(0, 0); serr != nil {
				return injectCancelReader(res, cancel), errors.Join(err, ErrSeekingBody)
			}
			req.Body = io.NopCloser(br)
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
