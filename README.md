# retryhttp [![Build Status](https://github.com/justinrixx/retryhttp/actions/workflows/go.yml/badge.svg?branch=main)](https://github.com/justinrixx/retryhttp/actions) [![Go Reference](https://pkg.go.dev/badge/github.com/justinrixx/retryhttp.svg)](https://pkg.go.dev/github.com/justinrixx/retryhttp)

`retryhttp` allows you to add HTTP retries to your service or application with no refactoring at all, just a few lines of configuration where your client is instantiated. This package's goals are:

- Make adding retries easy, with no refactor required (as stated above)
- [Provide a good starting point for retry behavior](./docs/default.md)
- [Make customizing retry behavior easy](./docs/options.md)
- [Allow for one-off behavior changes without needing multiple HTTP clients](./docs/options.md#example)
- 100% standard library, with no external dependencies (have a peek at `go.mod`)

## How it works

`retryhttp` exports a `Transport` struct which implements [the standard library's `http.RoundTripper` interface](https://pkg.go.dev/net/http#RoundTripper). By performing retries at the `http.RoundTripper` level, retries can be introduced to a service or script with just a few lines of configuration and no changes required to any of the code actually making HTTP requests. Regardless of which HTTP client you're using, it's very likely to have a configurable `http.RoundTripper`, which means this package can be integrated.

`http.RoundTripper`s are also highly composable. By default, this package uses `http.DefaultTransport` as its underlying `RoundTripper`, but you may choose to wrap a customized one that sets `MaxIdleConns`, or even [something like this](https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp#Transport) that captures metric spans to instrument your calls.

## Get it

```bash
$ go get github.com/justinrixx/retryhttp
```

## Example

```go
// BEFORE
client := http.Client{
    // HTTP client options
}

// AFTER
client := http.Client{
    Transport: retryhttp.New(
        // optional retry configurations
        retryhttp.WithShouldRetryFn(func(attempt retryhttp.Attempt) bool {
            return attempt.Res != nil && attempt.Res.StatusCode == http.StatusServiceUnavailable
        }),
        retryhttp.WithDelayFn(func(attempt retryhttp.Attempt) time.Duration {
            return expBackoff(attempt.Count)
        }),
        retryhttp.WithMaxRetries(2),
    ),
    // other HTTP client options
}
```

This package was inspired by https://github.com/PuerkitoBio/rehttp/ but it aims to take away a couple footguns, provide widely-applicable defaults, and make one-off overriding options easy using context keys.

## License

[MIT](https://github.com/justinrixx/retryhttp/blob/main/LICENSE)
