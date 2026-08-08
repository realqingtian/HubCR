package jobs

import (
	"errors"
	"testing"
)

func TestHandlerErrorClassificationDoesNotExposeCause(t *testing.T) {
	secret := errors.New("secret-bearing scanner output")
	err := Retryable("SCANNER_UNAVAILABLE", secret)
	if err.Error() != "job handler failed" || !errors.Is(err, secret) {
		t.Fatalf("Retryable() = %v", err)
	}
	code, terminal := ClassifyHandlerError(err)
	if code != "SCANNER_UNAVAILABLE" || terminal {
		t.Fatalf("ClassifyHandlerError() = %q, %v", code, terminal)
	}

	code, terminal = ClassifyHandlerError(Permanent("INVALID_ARTIFACT", secret))
	if code != "INVALID_ARTIFACT" || !terminal {
		t.Fatalf("ClassifyHandlerError(permanent) = %q, %v", code, terminal)
	}
}
