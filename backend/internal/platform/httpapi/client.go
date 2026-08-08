package httpapi

import (
	"net"
	"net/http"
)

// ClientKey returns a stable per-connection source key without trusting forwarded
// headers supplied by an untrusted direct client.
func ClientKey(request *http.Request) string {
	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err == nil {
		return host
	}
	return request.RemoteAddr
}
