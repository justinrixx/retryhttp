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

var ErrThrottled = errors.New("retry throttled")

// Throttler is an interface decides if a retry should be throttled
type Throttler interface {
	ShouldThrottle(res *http.Response, err error) bool
	Stop()
}

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
func (t defaultThrottler) ShouldThrottle(res *http.Response, err error, isRetry bool) bool {
	t.recordStats(res, err, isRetry)

	total := t.totalReqs.read()
	fTotal := float64(total)

	if isRetry {
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

// recordStats records information about a request which the throttler can use later
// to make throttling decisions. This throttler records 429s and context deadline
// exceeded errors as signal of overload.
func (t defaultThrottler) recordStats(res *http.Response, err error, isRetry bool) {
	t.totalReqs.increment()
	if isRetry {
		t.retriedReqs.increment()
	}

	if (err != nil && errors.Is(err, context.DeadlineExceeded)) ||
		(res != nil && res.StatusCode == http.StatusTooManyRequests) {
		t.overloadedReqs.increment()
	}
}

func (t defaultThrottler) Stop() {
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
