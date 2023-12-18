# Default behaviors

The default behaviors of this package are mostly implemented in `DefaultShouldRetryFn` and `DefaultDelayFn`, which you may choose to read for yourself. A high level description of their logic follows.

## `DefaultShouldRetryFn`

- If an error occured (non-nil `attempt.Err`, nil `attempt.Res`), and if that error is a DNS error, the request is retried. This is because it never reached the target server due to failing on the DNS lookup.
- If an error occured and if that error is a common timeout error (see `IsTimeoutErr`), the request is retried only if it is guessed to be idempotent[^1].
- If no error occured an a non-nil response was returned, the request is retried if the response indicates the server expects a retry[^2].
- If the request is guessed idempotent[^1] and the status code is 502 or 503, the request is retried
- Otherwise, the request is not retried

The methods considered idempotent and the status codes considered retryable can be tweaked by using `CustomizedShouldRetryFn` instead.

## `DefaultDelayFn`

- If the `Retry-After` header is provided, a wait duration is derived from its value. This field may be a non-negative integer representing seconds, or a timestamp. Once a duration is obtained, jitter of magnitude up to one third ($\frac{1}{3}$) is added or subtracted from that duration as jitter.
- If no `Retry-After` header is provided, exponential backoff with jitter is used. The algorithm used [is described here](https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/) as "full jitter". The exponential base used is 250ms, and it is capped at 10s.

The jitter magnitude, exponential base, and exponential backoff cap can be tweaked by using `CustomizedDelayFn` instead.

[^1]: A request is guessed idempotent if it uses an [idempotent HTTP method](-editor.org/rfc/rfc9110.html#name-idempotent-methods) or includes the `X-Idempotency-Key` or `Idempotency-Key` header.
[^2]: A status code of 429 indicates the server did not process the request and anticipates the caller to retry after some delay. Similarly, the `Retry-After` response header indicates the request should be retried after a delay.