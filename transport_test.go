package retryhttp_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/justinrixx/retryhttp"
)

func TestTransport_RoundTrip(t *testing.T) {
	type fields struct {
		tr             *retryhttp.Transport
		method         string
		body           io.Reader
		ctxFn          func(context.Context) context.Context
		responseCodes  func(int) int
		responseBodies func(int) []byte
	}
	tests := []struct {
		name             string
		fields           fields
		wantAttemptCount int
		wantStatus       int
		wantErr          bool
		expReqBody       []byte
		expResBody       []byte
	}{
		{
			name: "should retry the appropriate number of times with default configurations",
			fields: fields{
				tr: retryhttp.New(
					retryhttp.WithDelayFn(func(_ retryhttp.Attempt) time.Duration {
						return 0
					}),
				),
				method: http.MethodGet,
				responseCodes: func(_ int) int {
					return http.StatusTooManyRequests
				},
			},
			wantAttemptCount: 4,
			wantStatus:       http.StatusTooManyRequests,
		},
		{
			name: "should not retry on success",
			fields: fields{
				tr: retryhttp.New(
					retryhttp.WithDelayFn(func(_ retryhttp.Attempt) time.Duration {
						return 0
					}),
				),
				method: http.MethodGet,
				responseCodes: func(_ int) int {
					return http.StatusOK
				},
			},
			wantAttemptCount: 1,
			wantStatus:       http.StatusOK,
		},
		{
			name: "should not retry beyond success",
			fields: fields{
				tr: retryhttp.New(
					retryhttp.WithDelayFn(func(_ retryhttp.Attempt) time.Duration {
						return 0
					}),
				),
				method: http.MethodGet,
				responseCodes: func(i int) int {
					if i > 1 {
						return http.StatusOK
					}
					return http.StatusTooManyRequests
				},
			},
			wantAttemptCount: 3,
			wantStatus:       http.StatusOK,
		},
		{
			name: "should respect custom ShouldRetryFn",
			fields: fields{
				tr: retryhttp.New(
					retryhttp.WithShouldRetryFn(func(attempt retryhttp.Attempt) bool {
						return attempt.Res != nil && attempt.Res.StatusCode == http.StatusTeapot
					}),
					retryhttp.WithDelayFn(func(_ retryhttp.Attempt) time.Duration {
						return 0
					}),
				),
				method: http.MethodGet,
				responseCodes: func(i int) int {
					return http.StatusTeapot
				},
			},
			wantAttemptCount: 4,
			wantStatus:       http.StatusTeapot,
		},
		{
			name: "should respect custom MaxRetries",
			fields: fields{
				tr: retryhttp.New(
					retryhttp.WithMaxRetries(2),
					retryhttp.WithDelayFn(func(_ retryhttp.Attempt) time.Duration {
						return 0
					}),
				),
				method: http.MethodGet,
				responseCodes: func(_ int) int {
					return http.StatusTooManyRequests
				},
			},
			wantAttemptCount: 3,
			wantStatus:       http.StatusTooManyRequests,
		},
		{
			name: "should retry requests with bodies",
			fields: fields{
				tr: retryhttp.New(
					retryhttp.WithDelayFn(func(_ retryhttp.Attempt) time.Duration {
						return 0
					}),
				),
				method: http.MethodPost,
				body:   bytes.NewReader([]byte(`this is the request body`)),
				responseCodes: func(i int) int {
					if i < 2 {
						return http.StatusTooManyRequests
					}
					return http.StatusOK
				},
				responseBodies: func(i int) []byte {
					if i < 2 {
						return nil
					}
					return []byte(`foo bar baz it's all ok`)
				},
			},
			wantAttemptCount: 3,
			wantStatus:       http.StatusOK,
			expReqBody:       []byte(`this is the request body`),
			expResBody:       []byte(`foo bar baz it's all ok`),
		},
		{
			name: "should prevent retry of requests with bodies when enabled",
			fields: fields{
				tr: retryhttp.New(
					retryhttp.WithPreventRetryWithBody(true),
					retryhttp.WithDelayFn(func(_ retryhttp.Attempt) time.Duration {
						return 0
					}),
				),
				method: http.MethodPost,
				body:   bytes.NewReader([]byte(`this is the request body`)),
				responseCodes: func(i int) int {
					return http.StatusTooManyRequests
				},
			},
			wantAttemptCount: 1,
			wantStatus:       http.StatusTooManyRequests,
			expReqBody:       []byte(`this is the request body`),
		},
		{
			name: "should respect MaxRetries context key override",
			fields: fields{
				tr: retryhttp.New(
					retryhttp.WithMaxRetries(0), // transport says 0 retries
					retryhttp.WithDelayFn(func(_ retryhttp.Attempt) time.Duration {
						return 0
					}),
				),
				method: http.MethodGet,
				ctxFn: func(ctx context.Context) context.Context { // context overrides retry count
					return retryhttp.SetMaxRetries(ctx, 3)
				},
				responseCodes: func(_ int) int {
					return http.StatusTooManyRequests
				},
			},
			wantAttemptCount: 4,
			wantStatus:       http.StatusTooManyRequests,
		},
		{
			name: "should respect ShouldRetryFn context key override",
			fields: fields{
				tr: retryhttp.New(
					retryhttp.WithShouldRetryFn(func(_ retryhttp.Attempt) bool {
						return true
					}),
					retryhttp.WithDelayFn(func(_ retryhttp.Attempt) time.Duration {
						return 0
					}),
				),
				method: http.MethodGet,
				ctxFn: func(ctx context.Context) context.Context {
					return retryhttp.SetShouldRetryFn(ctx, func(_ retryhttp.Attempt) bool {
						return false
					})
				},
				responseCodes: func(_ int) int {
					return http.StatusOK
				},
			},
			wantAttemptCount: 1,
			wantStatus:       http.StatusOK,
		},
		{
			name: "should respect prevent retry with body context key override",
			fields: fields{
				tr: retryhttp.New(
					retryhttp.WithPreventRetryWithBody(true),
					retryhttp.WithDelayFn(func(_ retryhttp.Attempt) time.Duration {
						return 0
					}),
				),
				method: http.MethodPost,
				body:   bytes.NewReader([]byte(`this is the request body`)),
				ctxFn: func(ctx context.Context) context.Context {
					return retryhttp.SetPreventRetryWithBody(ctx, false)
				},
				responseCodes: func(i int) int {
					if i == 0 {
						return http.StatusTooManyRequests
					}
					return http.StatusOK
				},
			},
			wantAttemptCount: 2,
			wantStatus:       http.StatusOK,
			expReqBody:       []byte(`this is the request body`),
		},
		{
			name: "should retry the appropriate number of times with default DelayFn",
			fields: fields{
				// actually sleeps which makes tests longer
				tr: retryhttp.New(
					retryhttp.WithMaxRetries(1),
				),
				method: http.MethodGet,
				responseCodes: func(_ int) int {
					return http.StatusTooManyRequests
				},
			},
			wantAttemptCount: 2,
			wantStatus:       http.StatusTooManyRequests,
		},
		{
			name: "should retry the appropriate number of times with delayFn overridden to 0",
			fields: fields{
				tr:     retryhttp.New(),
				method: http.MethodGet,
				ctxFn: func(ctx context.Context) context.Context {
					return retryhttp.SetDelayFn(ctx, func(_ retryhttp.Attempt) time.Duration {
						return 0
					})
				},
				responseCodes: func(_ int) int {
					return http.StatusTooManyRequests
				},
			},
			wantAttemptCount: 4,
			wantStatus:       http.StatusTooManyRequests,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attemptCount := 0
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.expReqBody != nil {
					body, err := io.ReadAll(r.Body)
					if err != nil {
						t.Errorf("error reading request body stream: %s", err)
					}
					r.Body.Close()

					if !reflect.DeepEqual(body, tt.expReqBody) {
						t.Errorf("request body does not match expected. got %s, want %s", string(body), string(tt.expReqBody))
					}
				}

				w.WriteHeader(tt.fields.responseCodes(attemptCount))
				if tt.fields.responseBodies != nil {
					b := tt.fields.responseBodies(attemptCount)
					if b != nil {
						w.Write(b)
					}
				}

				attemptCount++
			}))
			defer ts.Close()

			client := http.Client{
				Transport: tt.fields.tr,
			}

			req, err := http.NewRequest(tt.fields.method, ts.URL, tt.fields.body)
			if err != nil {
				t.Errorf("error creating request: %s", err)
			}

			ctx := context.Background()
			if tt.fields.ctxFn != nil {
				ctx = tt.fields.ctxFn(ctx)
			}

			res, err := client.Do(req.WithContext(ctx))
			if (err != nil) != tt.wantErr {
				t.Errorf("Transport.RoundTrip() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if attemptCount != tt.wantAttemptCount {
				t.Errorf("unexpected attempt count: got %d, want %d", attemptCount, tt.wantAttemptCount)
			}

			if tt.wantStatus > 0 {
				if res == nil {
					t.Errorf("unexpected status: want %d, got nil response", tt.wantStatus)
				} else if res.StatusCode != tt.wantStatus {
					t.Errorf("unexpected status: got %d, want %d", res.StatusCode, tt.wantStatus)
				}
			}
			if tt.expResBody != nil {
				body, err := io.ReadAll(res.Body)
				if err != nil {
					t.Errorf("unexpected error reading response body: %s", err)
				}
				res.Body.Close()
				if !reflect.DeepEqual(body, tt.expResBody) {
					t.Errorf("unexpected response body: got %s, want %s", string(body), string(tt.expResBody))
				}
			}
		})
	}
}

func TestPerAttemptTimeout(t *testing.T) {
	mu := sync.Mutex{}
	attemptCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attemptCount++
		mu.Unlock()
		time.Sleep(time.Millisecond * 100) // sleep longer than the timeout
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer ts.Close()

	tr := retryhttp.New(
		retryhttp.WithDelayFn(func(_ retryhttp.Attempt) time.Duration {
			return 0
		}),
		retryhttp.WithAttemptTimeout(time.Millisecond*10), // really short timeout
	)

	client := http.Client{
		Transport: tr,
	}

	req, err := http.NewRequest(http.MethodGet, ts.URL, nil)
	if err != nil {
		t.Fatalf("error creating request: %s", err)
	}

	_, err = client.Do(req)
	mu.Lock()
	if attemptCount != 4 {
		t.Fatalf("attempt count does not match expected; got %d, want %d", attemptCount, 4)
	}
	if err == nil {
		t.Fatal("expected error from request but got nil")
	}

	attemptCount = 0
	mu.Unlock()

	// override per-attempt timeout with context
	ctx := retryhttp.SetAttemptTimeout(context.Background(), 0)
	res, err := client.Do(req.WithContext(ctx))
	mu.Lock()
	if attemptCount != 4 {
		t.Fatalf("attempt count does not match expected; got %d, want %d", attemptCount, 4)
	}
	if err != nil {
		t.Fatalf("expected nil error but got %s", err)
	}
	if res.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("unexpected status code; got %d, want %d", res.StatusCode, http.StatusTooManyRequests)
	}

	attemptCount = 0
	mu.Unlock()
}

// TODO test parent context expiring
func TestParentContextDeadline(t *testing.T) {
	mu := sync.Mutex{}
	attemptCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attemptCount++
		mu.Unlock()
		time.Sleep(time.Millisecond * 100) // sleep longer than the deadline
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer ts.Close()

	tr := retryhttp.New(
		retryhttp.WithDelayFn(func(_ retryhttp.Attempt) time.Duration {
			return 0
		}),
	)

	client := http.Client{
		Transport: tr,
	}

	req, err := http.NewRequest(http.MethodGet, ts.URL, nil)
	if err != nil {
		t.Fatalf("error creating request: %s", err)
	}

	// really short deadline
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Millisecond*10))
	defer cancel()
	_, err = client.Do(req.WithContext(ctx))
	mu.Lock()
	if attemptCount != 1 {
		t.Fatalf("attempt count does not match expected; got %d, want %d", attemptCount, 1)
	}
	if err == nil {
		t.Fatal("expected error from request but got nil")
	}
}
