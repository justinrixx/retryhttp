package retryhttp_test

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/justinrixx/retryhttp"
)

type timeoutErr struct{}

func (t timeoutErr) Error() string { return "timeout error" }
func (t timeoutErr) Timeout() bool { return true }

func TestIsDNSErr(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "returns true for a dns timeout error",
			err: &net.DNSError{
				IsTimeout: true,
			},
			want: true,
		},
		{
			name: "returns true for a dns not found error",
			err: &net.DNSError{
				IsNotFound: true,
			},
			want: true,
		},
		{
			name: "returns true for generic dns error",
			err:  &net.DNSError{},
			want: true,
		},
		{
			name: "returns false for non dns error",
			err:  errors.New("fake error"),
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := retryhttp.IsDNSErr(tt.err); got != tt.want {
				t.Errorf("IsDNSErr() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsTimeoutErr(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "returns true for an error that timed out",
			err: &net.OpError{
				Err: timeoutErr{},
			},
			want: true,
		},
		{
			name: "returns true for a context deadline exceeded error",
			err:  context.DeadlineExceeded,
			want: true,
		},
		{
			name: "returns false for non-timeout error",
			err:  errors.New("fake error"),
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := retryhttp.IsTimeoutErr(tt.err); got != tt.want {
				t.Errorf("IsTimeoutErr() = %v, want %v", got, tt.want)
			}
		})
	}
}
