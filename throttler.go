package retryhttp

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"net/http"
	"time"
)

const (
	defaultTimespanSeconds = 120                         // 2 mins
	defaultBuckets         = defaultTimespanSeconds / 10 // 10s per bucket
)

type (
	// Throttler is an interface decides if a retry should be throttled
	Throttler interface {
		ShouldThrottle(attempt Attempt) bool
		RecordStats(attempt Attempt)
		Stop()
	}
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
// retryBudget is the per-client retry budget described in the "Deciding to Retry"
// section of the same chapter.
//
// [this equation]: https://sre.google/sre-book/handling-overload/#eq2101
func NewDefaultThrottler(k float64, retryBudget float64) defaultThrottler {
	totalStop := make(chan bool)
	overloadStop := make(chan bool)
	retriedStop := make(chan bool)

	totalCounter := newAtomicCounter(totalStop, defaultBuckets, time.Second*defaultTimespanSeconds)
	overloadCounter := newAtomicCounter(overloadStop, defaultBuckets, time.Second*defaultTimespanSeconds)
	retriedCounter := newAtomicCounter(retriedStop, defaultBuckets, time.Second*defaultTimespanSeconds)

	return defaultThrottler{
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
