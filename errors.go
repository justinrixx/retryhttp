package retryhttp

import (
	"errors"
	"net"
)

// IsDNSErr is used to determine if an error from an attempt is due to DNS. Requests that
// failed with a DNS error
func IsDNSErr(err error) bool {
	var dnse *net.DNSError
	return errors.As(err, &dnse)
}

// IsTimeoutErr is used to determine if an error from an attempt is due to a common timeout.
// This includes network timeouts or the context deadline being exceeded.
func IsTimeoutErr(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
