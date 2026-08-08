package authlimit

import (
	"context"

	"hubcr.io/hubcr/internal/modules/auth"
)

// AllowAll is available only to tests that are not exercising authentication admission.
type AllowAll struct{}

func (AllowAll) Allow(context.Context, auth.LoginAttempt) error { return nil }
