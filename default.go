package retryhttp

import (
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"time"
)

// prng generates random numbers for calculating jitter
var prng = rand.New(rand.NewSource(time.Now().UnixNano()))

// CustomizedShouldRetryFnOptions are used to tweak the behavior of CustomizedShouldRetryFn.
type CustomizedShouldRetryFnOptions struct {
	IdempotentMethods    []string
	RetryableStatusCodes []int
}

// CustomizedDelayFnOptions are used to tweak the behavior of [CustomizedDelayFn].
// Base and Cap are used in calculating exponential backoff: min(base * (2 ** i), cap)
// JitterMagnitude determines the maximum portion of delay specified by Retry-After to
// add or subtract as jitter.
// [DefaultDelayFn] uses base=250ms, cap=10s, jitter magnitude=0.333
type CustomizedDelayFnOptions struct {
	Base            time.Duration
	Cap             time.Duration
	JitterMagnitude float64
}

// DefaultShouldRetryFn is a sane default starting point for a should retry policy.
// Not all HTTP requests should be retried. If a request succeeded or failed in a way
// that is not likely to change on retry (is deterministic), a retry is wasteful.
// Idempotency should also be taken into account when retrying: retrying a non-idempotent
// request can result in creating duplicate resources for example.
// DefaultShouldRetryFn's behavior is that:
//   - DNS errors never reached the target server, and are therefore safe to retry.
//   - If a timeout error occurred and the request is guessed to be idempotent, it is retried.
//   - If a 429 status is returned or the Retry-After response header is included it is retried.
//   - If the status code is retryable and the request is guessed to be idempotent it is retried.
//
// Default retryablestatus codes are http.StatusBadGateway and http.StatusServiceUnavailable.
// Idempotency is guessed based on the inclusion of the Idempotency-Key or X-Idempotency-Key
// header, or an idempotent method (as defined in RFC 9110).
var DefaultShouldRetryFn = CustomizedShouldRetryFn(CustomizedShouldRetryFnOptions{
	// https://www.rfc-editor.org/rfc/rfc9110.html#name-idempotent-methods
	IdempotentMethods: []string{
		http.MethodGet,
		http.MethodHead,
		http.MethodOptions,
		http.MethodTrace,
		http.MethodPut,
		http.MethodDelete,
	},
	RetryableStatusCodes: []int{http.StatusBadGateway, http.StatusServiceUnavailable},
})

// CustomizedShouldRetryFn has the same logic as [DefaultShouldRetryFn] but it allows for
// specifying which status codes should be assumed retryable and which methods should be
// guessed idempotent. This is useful if the default behavior is desired, with small tweaks.
func CustomizedShouldRetryFn(options CustomizedShouldRetryFnOptions) func(attempt Attempt) bool {
	idempotentMethods := map[string]bool{}
	retryableStatusCodes := map[int]bool{}

	for _, method := range options.IdempotentMethods {
		idempotentMethods[method] = true
	}
	for _, status := range options.RetryableStatusCodes {
		retryableStatusCodes[status] = true
	}

	return func(attempt Attempt) bool {
		idempotent := guessIdempotent(attempt.Req, idempotentMethods)

		if attempt.Err != nil {
			// dns errors are safe to retry
			if IsDNSErr(attempt.Err) {
				return true
			}

			return idempotent && IsTimeoutErr(attempt.Err)
		}

		// caller signalling they expect a retry
		if attempt.Res.StatusCode == http.StatusTooManyRequests || attempt.Res.Header.Get("Retry-After") != "" {
			return true
		}

		return idempotent && retryableStatusCodes[attempt.Res.StatusCode]
	}
}

// DefaultDelayFn is a sane default starting point for a delay policy. It respects
// the [Retry-After] response header if present. This header is used by the destination
// service to communicate when the next attempt is appropriate. It can be either
// an integer (specifying the number of seconds to wait) or a timestamp from which
// a duration is calculated. Once a base duration is determined, plus or minus up to
// 1/3 of that value is added as jitter.
// If the Retry-After header is not present, the "[full jitter]" exponential backoff
// algorithm is used with base=250ms and cap=10s.
//
// [Retry-After]: https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Retry-After
// [full jitter]: https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/
var DefaultDelayFn = CustomizedDelayFn(CustomizedDelayFnOptions{
	Base:            time.Millisecond * 250,
	Cap:             time.Second * 10,
	JitterMagnitude: 0.333,
})

// CustomizedDelayFn has the same logic as [DefaultDelayFn] but it allows for specifying
// the exponential backoff's base and maximum, as well as the fraction to calculate
// jitter with.
func CustomizedDelayFn(options CustomizedDelayFnOptions) func(attempt Attempt) time.Duration {
	return func(attempt Attempt) time.Duration {
		// check for a retry-after header
		if attempt.Res != nil && attempt.Res.Header.Get("Retry-After") != "" {
			retryAfterStr := attempt.Res.Header.Get("Retry-After")

			// try parsing as an integer
			// https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Retry-After#delay-seconds
			i, err := strconv.Atoi(retryAfterStr)
			if err == nil {
				return addJitter(time.Duration(i)*time.Second, options.JitterMagnitude)
			}

			// try parsing as date
			// https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Retry-After#http-date
			t, err := time.Parse(http.TimeFormat, retryAfterStr)
			if err == nil {
				return addJitter(time.Until(t), options.JitterMagnitude)
			}
		}

		// fall back to exponential backoff
		return expBackoff(attempt.Count, options.Base, options.Cap)
	}
}

// based on "full jitter": https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/
func expBackoff(attempt int, base time.Duration, cap time.Duration) time.Duration {
	exp := math.Pow(2, float64(attempt-1))
	v := float64(base) * exp
	return time.Duration(
		prng.Int63n(int64(math.Min(float64(cap), v))),
	)
}

// default jitter is plus or minus 1/3 of the duration
func addJitter(d time.Duration, magnitude float64) time.Duration {
	f := float64(d)
	mj := f * magnitude

	// randomness determines jitter magnitude
	j := prng.Float64() * mj

	// randomness determines if jitter is added or subtracted
	coin := prng.Float64()
	if coin < 0.5 {
		return time.Duration(f + j)
	}

	return time.Duration(f - j)
}

func guessIdempotent(req *http.Request, idempotentMethods map[string]bool) bool {
	if req.Header.Get("Idempotency-Key") != "" || req.Header.Get("X-Idempotency-Key") != "" {
		return true
	}

	return idempotentMethods[req.Method]
}
