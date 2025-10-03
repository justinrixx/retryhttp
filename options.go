package retryhttp

import (
	"context"
	"net/http"
	"time"
)

type (
	maxRetriesContextKeyType           string
	shouldRetryFnContextKeyType        string
	delayFnContextKeyType              string
	preventRetryWithBodyContextKeyType string
	attemptTimeoutContextKeyType       string
)

const (
	maxRetriesContextKey           = maxRetriesContextKeyType("maxRetries")
	shouldRetryFnContextKey        = shouldRetryFnContextKeyType("shouldRetryFn")
	delayFnContextKey              = delayFnContextKeyType("delayFn")
	preventRetryWithBodyContextKey = preventRetryWithBodyContextKeyType("preventRetryWithBody")
	attemptTimeoutContextKey       = attemptTimeoutContextKeyType("attemptTimeout")
)

// WithTransport configures a Transport with an internal roundtripper of its own.
// This is often [http.DefaultTransport], but it could be anything else.
func WithTransport(transport http.RoundTripper) func(*Transport) {
	return func(t *Transport) {
		t.rt = transport
	}
}

// WithMaxRetries configures the maximum number of retries a Transport is allowed to make.
// If not set, defaults to [DefaultMaxRetries]. Note that this number does not include the
// initial attempt, so if this is configured as 3, there could be up to 4 total attempts.
func WithMaxRetries(maxRetries int) func(*Transport) {
	return func(t *Transport) {
		t.maxRetries = &maxRetries
	}
}

// WithShouldRetryFn configures the [ShouldRetryFn] callback to use.
func WithShouldRetryFn(shouldRetryFn ShouldRetryFn) func(*Transport) {
	return func(t *Transport) {
		t.shouldRetryFn = shouldRetryFn
	}
}

// WithDelayFn configures the [DelayFn] callback to use.
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

// SetMaxRetries can be used to override the settings on a Transport.
// Any request made with the returned context will have its MaxRetries setting
// overridden with the provided value.
func SetMaxRetries(ctx context.Context, maxRetries int) context.Context {
	return context.WithValue(ctx, maxRetriesContextKey, maxRetries)
}

// SetShouldRetryFn can be used to override the settings on a Transport.
// Any request made with the returned context will have its [ShouldRetryFn] overridden with
// the provided value.
func SetShouldRetryFn(ctx context.Context, shouldRetryFn ShouldRetryFn) context.Context {
	return context.WithValue(ctx, shouldRetryFnContextKey, shouldRetryFn)
}

// SetDelayFn can be used to override the settings on a Transport.
// Any request made with the returned context will have its [DelayFn] overridden with
// the provided value.
func SetDelayFn(ctx context.Context, delayFn DelayFn) context.Context {
	return context.WithValue(ctx, delayFnContextKey, delayFn)
}

// SetPreventRetryWithBody can be used to override the settings on a
// Transport. Any request made with the returned context will have its
// PreventRetryWithbody setting overridden with the provided value.
func SetPreventRetryWithBody(ctx context.Context, preventRetryWithBody bool) context.Context {
	return context.WithValue(ctx, preventRetryWithBodyContextKey, preventRetryWithBody)
}

// SetAttemptTimeout can be used to override the settings on a// Transport.
// Any request made with the returned context will have its AttemptTimeout setting
// overridden with the provided value.
func SetAttemptTimeout(ctx context.Context, attemptTimeout time.Duration) context.Context {
	return context.WithValue(ctx, attemptTimeoutContextKey, attemptTimeout)
}

func getMaxRetriesFromContext(ctx context.Context) (int, bool) {
	val, ok := ctx.Value(maxRetriesContextKey).(int)
	return val, ok
}

func getShouldRetryFnFromContext(ctx context.Context) (ShouldRetryFn, bool) {
	val, ok := ctx.Value(shouldRetryFnContextKey).(ShouldRetryFn)
	return val, ok
}

func getDelayFnFromContext(ctx context.Context) (DelayFn, bool) {
	val, ok := ctx.Value(delayFnContextKey).(DelayFn)
	return val, ok
}

func getPreventRetryWithBodyFromContext(ctx context.Context) (bool, bool) {
	val, ok := ctx.Value(preventRetryWithBodyContextKey).(bool)
	return val, ok
}

func getAttemptTimeoutFromContext(ctx context.Context) (time.Duration, bool) {
	val, ok := ctx.Value(attemptTimeoutContextKey).(time.Duration)
	return val, ok
}
