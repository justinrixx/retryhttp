package retryhttp

import (
	"context"
	"io"
	"net/http"
)

// If a request's context is canceled before the response's body is read, the byte stream
// will be reclaimed by the runtime. This results in a race against the runtime to read
// the body and often ends in an error. Instead of canceling the context before returning
// a response out, the cancel call is delayed until Close is called on the response body.
// The response body is replaced with this struct to facilitate this.
// This solution is based on https://github.com/go-kit/kit/issues/773.
type cancelReader struct {
	io.ReadCloser

	cancel context.CancelFunc
}

func (cr cancelReader) Close() error {
	cr.cancel()
	return cr.ReadCloser.Close()
}

func injectCancelReader(res *http.Response, cancel context.CancelFunc) *http.Response {
	if res == nil {
		return nil
	}

	res.Body = cancelReader{
		ReadCloser: res.Body,
		cancel:     cancel,
	}
	return res
}
