# retryhttp

`retryhttp` exports a `Transport` struct which implements [the standard library's `http.RoundTripper` interface](https://pkg.go.dev/net/http#RoundTripper). By performing retries at the `http.RoundTripper` level, retries can be introduced to a service or script with just a few lines of configuration and no changes required to any of the code making HTTP requests. Regardless of which HTTP client you're using, it's very likely to have a configurable `http.RoundTripper`, which means this package can be integrated.

## Example

```go
// BEFORE
client := http.Client{
    // options
}

// AFTER
transport := retryhttp.New(
    // optional configurations
)

client := http.Client{
    Transport: transport,
    // other options
}
```

This package was inspired by https://github.com/PuerkitoBio/rehttp/ but it aims to take away a couple footguns, provide widely-applicable defaults, and make one-off overriding options easy using context keys.

## TODO

This package is still in development and it is not recommended for use yet.

- ~~`Transport` with `New` constructor and configurable options~~
- ~~Support for per-attempt timeouts~~
- ~~Ability to override options using context keys~~
- Good default `ShouldRetryFn` and `DelayFn`
- Helper functions for recognizing common HTTP errors
- Tweakable versions of the default `ShouldRetryFn` and `DelayFn`
- Unit tests
- Automated test runs in workflow

## License

[MIT](https://github.com/justinrixx/retryhttp/blob/main/LICENSE)
