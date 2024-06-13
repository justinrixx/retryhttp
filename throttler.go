package retryhttp

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"net/http"
	"time"
)

type (
	// Throttler is an interface which decides if a retry should be throttled. This is
	// useful for easing the pressure on an overloaded dependency, allowing it to more
	// effectively make progress and recover.
	//
	// In order for most throttlers to make valid decisions, some amount of data must
	// be recorded about previous requests and their results. For example, the throttler
	// provided by [NewDefaultThrottler] tracks the total number of requests made, the
	// number of requests which received an indicator of overload, and the total number
	// of retries made. These counts are maintained across a 2 minute sliding window.
	// RecordStats is called each time a response is received, but before consulting
	// the [ShouldRetryFn] or [Throttler.ShouldThrottle] (and either performing another
	// attempt or returning the response as-is).
	//
	// ShouldThrottle is called before each attempt. Note that this is per attempt, not
	// per retry, which means the initial (non-retry) request has the potential to be
	// throttled. Throttlers can use the [Attempt.Count] to determine if an attempt is
	// a retry or the initial request if they choose to. Note that [Attempt.Count] begins
	// at zero.
	Throttler interface {
		// ShouldThrottle decides whether a request should be throttled before it is
		// made. It is called before each attempt (including the initial one).
		ShouldThrottle(attempt Attempt) bool

		// RecordStats is an opportunity for the throttler to record information about
		// requests made and their responses to use for future decisions. ShouldThrottle
		// is inadequate because the response to the call to ShouldThrottle for the
		// initial request is unknown at the time it is called.
		RecordStats(attempt Attempt)
	}
)

// noopthrottler records no stats and never throttles
type noopThrottler struct{}

func (*noopThrottler) ShouldThrottle(_ Attempt) bool {
	return false
}

func (*noopThrottler) RecordStats(_ Attempt) {}

const (
	defaultTimespanSeconds = 120                         // 2 mins
	defaultBuckets         = defaultTimespanSeconds / 10 // 10s per bucket
)

type defaultThrottler struct {
	totalReqs      *atomicCounter
	overloadedReqs *atomicCounter
	retriedReqs    *atomicCounter
	k              float64
	retryBudget    float64

	totalStop    chan<- bool
	overloadStop chan<- bool
	retriedStop  chan<- bool
}

// NewDefaultThrottler constructs a new parameterized throttler. k is described in
// [this equation].
//
// retryBudget is the per-client retry budget described in the "Deciding to Retry"
// section of the same chapter.
//
// The default throttler makes no attempt to segment requests into different
// dependencies, so it is recommended that one [Transport] be used per dependency.
//
// To avoid leaking memory and compute resources, be sure to call Stop on teardown.
//
// [this equation]: https://sre.google/sre-book/handling-overload/#eq2101
func NewDefaultThrottler(k float64, retryBudget float64) Throttler {
	totalStop := make(chan bool)
	overloadStop := make(chan bool)
	retriedStop := make(chan bool)

	totalCounter := newAtomicCounter(totalStop, defaultBuckets, time.Second*defaultTimespanSeconds)
	overloadCounter := newAtomicCounter(overloadStop, defaultBuckets, time.Second*defaultTimespanSeconds)
	retriedCounter := newAtomicCounter(retriedStop, defaultBuckets, time.Second*defaultTimespanSeconds)

	return &defaultThrottler{
		totalReqs:      totalCounter,
		overloadedReqs: overloadCounter,
		retriedReqs:    retriedCounter,
		k:              k,
		retryBudget:    retryBudget,
	}
}

// ShouldThrottle decides whether a request should be throttled.
func (t *defaultThrottler) ShouldThrottle(attempt Attempt) bool {
	total := t.totalReqs.read()
	fTotal := float64(total)

	if isAttemptRetry(attempt) {
		// check per-client retry budget
		retried := t.retriedReqs.read()
		if float64(retried)/fTotal > t.retryBudget {
			return false
		}
	}

	// https://sre.google/sre-book/handling-overload/#eq2101
	overloaded := t.overloadedReqs.read()
	fAccepts := float64(total - overloaded)
	p := math.Max(0, (fTotal-(t.k*fAccepts))/(fTotal+1))

	return p > rand.Float64()
}

// RecordStats records information about a request which the throttler can use later
// to make throttling decisions. This throttler records 429s and context deadline
// exceeded errors as signals of overload.
func (t *defaultThrottler) RecordStats(attempt Attempt) {
	t.totalReqs.increment()
	if isAttemptRetry(attempt) {
		t.retriedReqs.increment()
	}

	if (attempt.Err != nil && errors.Is(attempt.Err, context.DeadlineExceeded)) ||
		(attempt.Res != nil && attempt.Res.StatusCode == http.StatusTooManyRequests) {
		t.overloadedReqs.increment()
	}
}

// Stop performs necessary cleanup of the throttler. Neglecting to call this will result
// in leaked memory and CPU resources.
func (t *defaultThrottler) Stop() {
	go func() {
		t.totalStop <- true
	}()
	go func() {
		t.overloadStop <- true
	}()
	go func() {
		t.retriedStop <- true
	}()
}

func isAttemptRetry(attempt Attempt) bool {
	return attempt.Count > 1
}
