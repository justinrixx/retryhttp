package retryhttp

import "net/http"

// CustomizedShouldRetryFnOptions are used to tweak the behavior of CustomizedShouldRetryFn.
type CustomizedShouldRetryFnOptions struct {
	IdempotentMethods    []string
	RetryableStatusCodes []int
}

// DefaultShouldRetryFn is a sane default starting point for a should retry policy.
// Not all HTTP requests should be retried. If a request succeeded or failed in a way
// that is not likely to change on retry (is deterministic), a retry is wasteful.
// Idempotency should also be taken into account when retrying: retrying a non-idempotent
// request can result in creating duplicate resources for example.
// DefaultShouldRetryFn's behavior is that:
// - DNS errors never reached the target server, and are therefore safe to retry
// - If a timeout error occurred and the request is guessed to be idempotent, it is retried
// - If a 429 status is returned or the Retry-After response header is included it is retried
// - If the status code is retryable and the request is guessed to be idempotent it is retried
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

// CustomizedShouldRetryFn has the same logic as DefaultShouldRetryFn but it allows for
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

func guessIdempotent(req *http.Request, idempotentMethods map[string]bool) bool {
	if req.Header.Get("Idempotency-Key") != "" || req.Header.Get("X-Idempotency-Key") != "" {
		return true
	}

	return idempotentMethods[req.Method]
}
