package httpapi

import (
	"net/http/httptest"
	"testing"
)

func TestClientKeyUsesRemoteAddressAndDoesNotTrustForwardedInput(t *testing.T) {
	request := httptest.NewRequest("GET", "/", nil)
	request.RemoteAddr = "192.0.2.10:4321"
	request.Header.Set("X-Forwarded-For", "198.51.100.20")
	if got := ClientKey(request); got != "192.0.2.10" {
		t.Fatalf("ClientKey() = %q, want direct peer address", got)
	}
}
