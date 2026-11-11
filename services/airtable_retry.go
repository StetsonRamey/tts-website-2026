package services

// Retry wrapper for requests to the exe.dev Airtable proxy.
//
// The proxy (airtable.int.exe.xyz → 169.254.169.254) can be unreachable for a
// few seconds. A failed dial means the request never left this machine, so it is
// safe to retry for any method. GETs are also retried on gateway errors
// (502/503/504) since they are idempotent.

import (
	"errors"
	"net"
	"net/http"
	"time"
)

var (
	airtableRetryDelays = []time.Duration{250 * time.Millisecond, 750 * time.Millisecond, 1500 * time.Millisecond}
	airtableSleep       = time.Sleep
)

// DoAirtable sends req with http.DefaultClient, retrying briefly if the
// Airtable proxy is temporarily unreachable.
func DoAirtable(req *http.Request) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		resp, err := http.DefaultClient.Do(req)
		retry := false
		switch {
		case err != nil:
			var opErr *net.OpError
			retry = errors.As(err, &opErr) && opErr.Op == "dial"
		case req.Method == http.MethodGet &&
			(resp.StatusCode == http.StatusBadGateway ||
				resp.StatusCode == http.StatusServiceUnavailable ||
				resp.StatusCode == http.StatusGatewayTimeout):
			retry = true
		}
		if !retry || attempt >= len(airtableRetryDelays) {
			return resp, err
		}
		if resp != nil {
			resp.Body.Close()
		}
		airtableSleep(airtableRetryDelays[attempt])
		if req.GetBody != nil {
			body, bodyErr := req.GetBody()
			if bodyErr != nil {
				return nil, bodyErr
			}
			req.Body = body
		}
	}
}

// doAirtableGet is a retrying replacement for http.Get against the proxy.
func doAirtableGet(rawURL string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	return DoAirtable(req)
}
