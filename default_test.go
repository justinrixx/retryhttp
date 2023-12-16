package retryhttp_test

import (
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/justinrixx/retryhttp"
)

func TestDefaultShouldRetryFn(t *testing.T) {
	tests := []struct {
		name    string
		attempt retryhttp.Attempt
		want    bool
	}{
		{
			name: "should retry on dns error for a GET request",
			attempt: retryhttp.Attempt{
				Count: 1,
				Req: &http.Request{
					Method: http.MethodGet,
				},
				Err: &net.DNSError{
					IsNotFound: true,
				},
			},
			want: true,
		},
		{
			name: "should retry on dns error for a POST request",
			attempt: retryhttp.Attempt{
				Count: 1,
				Req: &http.Request{
					Method: http.MethodPost,
				},
				Err: &net.DNSError{
					IsNotFound: true,
				},
			},
			want: true,
		},
		{
			name: "should retry idempotent requests that timed out",
			attempt: retryhttp.Attempt{
				Count: 1,
				Req: &http.Request{
					Method: http.MethodGet,
				},
				Err: &net.OpError{
					Err: timeoutErr{},
				},
			},
			want: true,
		},
		{
			name: "should not retry non-idempotent requests that timed out",
			attempt: retryhttp.Attempt{
				Count: 1,
				Req: &http.Request{
					Method: http.MethodPost,
				},
				Err: &net.OpError{
					Err: timeoutErr{},
				},
			},
			want: false,
		},
		{
			name: "should recognize requests with idempotency key headers as idempotent",
			attempt: retryhttp.Attempt{
				Count: 1,
				Req: &http.Request{
					Method: http.MethodPost,
					Header: http.Header{"Idempotency-Key": []string{"foobar"}},
				},
				Err: &net.OpError{
					Err: timeoutErr{},
				},
			},
			want: true,
		},
		{
			name: "should retry on 429 status",
			attempt: retryhttp.Attempt{
				Count: 1,
				Req: &http.Request{
					Method: http.MethodGet,
				},
				Res: &http.Response{
					StatusCode: http.StatusTooManyRequests,
				},
			},
			want: true,
		},
		{
			name: "should retry on 429 status even for non-idempotent methods",
			attempt: retryhttp.Attempt{
				Count: 1,
				Req: &http.Request{
					Method: http.MethodPost,
				},
				Res: &http.Response{
					StatusCode: http.StatusTooManyRequests,
				},
			},
			want: true,
		},
		{
			name: "should retry if retry-after header is present",
			attempt: retryhttp.Attempt{
				Count: 1,
				Req: &http.Request{
					Method: http.MethodGet,
				},
				Res: &http.Response{
					StatusCode: http.StatusInternalServerError,
					Header:     http.Header{"Retry-After": []string{"3"}},
				},
			},
			want: true,
		},
		{
			name: "should retry if retry-after header is present even for non-idempotent methods",
			attempt: retryhttp.Attempt{
				Count: 1,
				Req: &http.Request{
					Method: http.MethodPost,
				},
				Res: &http.Response{
					StatusCode: http.StatusInternalServerError,
					Header:     http.Header{"Retry-After": []string{"3"}},
				},
			},
			want: true,
		},
		{
			name: "should not retry if status is not retryable even if guessed idempotent",
			attempt: retryhttp.Attempt{
				Count: 1,
				Req: &http.Request{
					Method: http.MethodGet,
				},
				Res: &http.Response{
					StatusCode: http.StatusInternalServerError,
				},
			},
			want: false,
		},
		{
			name: "should not retry if request is guessed non-idempotent, even if status code is retryable",
			attempt: retryhttp.Attempt{
				Count: 1,
				Req: &http.Request{
					Method: http.MethodPost,
				},
				Res: &http.Response{
					StatusCode: http.StatusServiceUnavailable,
				},
			},
			want: false,
		},
		{
			name: "should not retry if request is guessed non-idempotent, or status code is not retryable",
			attempt: retryhttp.Attempt{
				Count: 1,
				Req: &http.Request{
					Method: http.MethodPost,
				},
				Res: &http.Response{
					StatusCode: http.StatusNotFound,
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := retryhttp.DefaultShouldRetryFn(tt.attempt)
			if actual != tt.want {
				t.Errorf("actual != expected: got %t, want %t", actual, tt.want)
			}
		})
	}
}

func TestDefaultDelayFn(t *testing.T) {
	tests := []struct {
		name       string
		retryAfter string
		attempt    int
		wantLow    time.Duration
		wantHigh   time.Duration
	}{
		{
			name:       "should respect retry-after when provided as 1s",
			retryAfter: "1",
			attempt:    1,
			wantLow:    time.Millisecond * 666,
			wantHigh:   time.Millisecond * 1333,
		},
		{
			name:       "should respect retry-after when provided as 2s",
			retryAfter: "2",
			attempt:    1,
			wantLow:    time.Millisecond * 1333,
			wantHigh:   time.Millisecond * 2666,
		},
		{
			name:       "should respect retry-after when provided as 10s",
			retryAfter: "10",
			attempt:    1,
			wantLow:    time.Millisecond * 6666,
			wantHigh:   time.Millisecond * 13333,
		},
		{
			name:       "should respect retry-after when provided as date 2s in the future",
			retryAfter: time.Now().UTC().Add(time.Second * 2).Format(http.TimeFormat),
			attempt:    1,
			wantLow:    time.Millisecond * 666,
			wantHigh:   time.Millisecond * 2666,
		},
		{
			name:       "should respect retry-after when provided as date 10s in the future",
			retryAfter: time.Now().UTC().Add(time.Second * 10).Format(http.TimeFormat),
			attempt:    1,
			wantLow:    time.Millisecond * 5555,
			wantHigh:   time.Millisecond * 13333,
		},
		{
			name:       "should respect retry-after when provided as date 2h in the past",
			retryAfter: time.Now().UTC().Add(time.Hour * -2).Format(http.TimeFormat),
			attempt:    1,
			wantLow:    time.Minute * -160,
			wantHigh:   time.Minute * -80,
		},
		// retry-after with non-numeric / non-date value
		{
			name:       "should fall back to exponential backoff when retry-after header is malformed",
			retryAfter: "not a date",
			attempt:    1,
			wantLow:    0,
			wantHigh:   time.Millisecond * 250,
		},
		// exp backoff with varying values
		{
			name:     "should return result consistent with exponential backoff on attempt 1",
			attempt:  1,
			wantLow:  0,
			wantHigh: time.Millisecond * 250,
		},
		{
			name:     "should return result consistent with exponential backoff on attempt 2",
			attempt:  2,
			wantLow:  0,
			wantHigh: time.Millisecond * 500,
		},
		{
			name:     "should return result consistent with exponential backoff on attempt 3",
			attempt:  3,
			wantLow:  0,
			wantHigh: time.Second,
		},
		{
			name:     "should return result consistent with exponential backoff on attempt 4",
			attempt:  4,
			wantLow:  0,
			wantHigh: time.Second * 2,
		},
		{
			name:     "should return result consistent with exponential backoff on attempt 5",
			attempt:  5,
			wantLow:  0,
			wantHigh: time.Second * 4,
		},
		{
			name:     "should return result consistent with exponential backoff on attempt 6",
			attempt:  6,
			wantLow:  0,
			wantHigh: time.Second * 8,
		},
		{
			name:     "exponential backoff should be capped by attempt 7",
			attempt:  7,
			wantLow:  0,
			wantHigh: time.Second * 10,
		},
		{
			name:     "exponential backoff should be capped beyond attempt 7",
			attempt:  100,
			wantLow:  0,
			wantHigh: time.Second * 10,
		},
		{
			name:     "exponential backoff should be capped beyond attempt 7",
			attempt:  100,
			wantLow:  0,
			wantHigh: time.Second * 10,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := http.Response{
				Header: http.Header{},
			}
			if tt.retryAfter != "" {
				res.Header.Set("Retry-After", tt.retryAfter)
			}
			actual := retryhttp.DefaultDelayFn(retryhttp.Attempt{
				Count: tt.attempt,
				Res:   &res,
			})
			if actual < tt.wantLow {
				t.Errorf("actual less than expected range; expected between %s and %s, got %s", tt.wantLow, tt.wantHigh, actual)
			}
			if actual > tt.wantHigh {
				t.Errorf("actual greater than expected range; expected between %s and %s, got %s", tt.wantLow, tt.wantHigh, actual)
			}
		})
	}
}
