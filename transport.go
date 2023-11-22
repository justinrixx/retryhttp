package retryhttp

import "net/http"

type Transport struct{}

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, nil
}
