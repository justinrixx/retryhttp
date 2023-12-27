# Options

`retryhttp.Transport` can be customized with several options. In general, each option that can be specified at creation time has an equivalent helper function for overriding the option using the request `Context`. An option set on the `Context` takes precedence over an option set on the `Transport`.

| Option | Context Equivalent | Default Value | Description |
| ------ | ------------------ | ------------- | ----------- |
| `WithTransport` | none | `http.DefaultTransport` | The internal `http.RoundTripper` to use for requests. |
| `WithShouldRetryFn` | `SetShouldRetryFn` | `DefaultShouldRetryFn` | The `ShouldRetryFn` that determines if a request should be retried. `DefaultShouldRetryFn` is a good starting point. If you're only looking to make minor tweaks,  `CustomizedShouldRetryFn` may be appropriate. |
| `WithDelayFn` | `SetDelayFn` | `DefaultDelayFn` | The `DelayFn` that determines how long to delay between retries. If `DefaultDelayFn` doesn't solve your use-case, `CustomizedDelayFn` may be appropriate. |
| `WithMaxRetries` | `SetMaxRetries` | 3 | The maximum number of retries to make. Note that this is the number of _retries_ not _attempts_, so a `MaxRetries` of 3 means up to 4 total attempts: 1 initial attempt and 3 retries. Note also that if your `ShouldRetryFn` returns `false`, a retry will not be made even if `MaxRetries` has not been exhausted. |
| `WithPreventRetryWithBody` | `SetPreventRetryWithBody` | `false` | Whether to prevent retrying requests that have a HTTP body. Any request that has any chance of needing a retry must buffer its body into memory so that it can be replayed in subsequent attempts. This may or may not be appropriate for certain use-cases, which is why this option is provided. |
| `WithAttemptTimeout` | `SetAttemptTimeout` | No timeout | A per-attempt timeout to be used. This differs from an overall timeout in that the timeout is reset for each attempt. Without a per-attempt timeout, the overall timeout could be exhausted in a single attempt with no time left for subsequent retries. Providing `time.Duration(0)` here removes the timeout. |

## Example

```go
client := http.Client{
    Transport: retryhttp.New(
        retryhttp.WithShouldRetryFn(attempt retryhttp.Attempt) bool {
            // only retry HTTP 418 statuses
            if attempt.Res != nil && attempt.Res.StatusCode == http.StatusTeapot {
                return true
            }
            return false
        },
        retryhttp.WithMaxRetries(2),
        retryhttp.WithAttemptTimeout(time.Second * 10),
    ),
}

ctx := context.TODO
ctx = retryhttp.SetShouldRetryFn(ctx, func(attempt retryhttp.Attempt) bool {
    // retry any error
    if attempt.Err != nil {
        return true
    }
    return false
})
ctx = retryhttp.SetMaxRetries(ctx, 1) // only 1 retry
ctx = retryhttp.SetAttemptTimeout(ctx, 0) // remove attempt timeout

req, err := http.NewRequest(http.MethodGet, "example.com", nil)
...

// add augmented context to the request: retries will abide by the overrides
// instead of the orginial configurations
res, err := client.Do(req.WithContext(ctx))
...
```
