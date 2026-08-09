// Package trustpolicyhandler exposes the authorized trust-policy management API for a
// namespace: read the current (highest-version) policy and create a new append-only
// version. Read access reuses the ViewOrganization capability; create is owner-only
// (ManageTrustPolicy). Per the M5-01 design, creating a version eagerly triggers
// namespace-scoped re-verification, and the response is minimal (no async-work counts).
package trustpolicyhandler

import (
	"context"
	"errors"
	"net/http"

	"hubcr.io/hubcr/internal/modules/auth"
	"hubcr.io/hubcr/internal/modules/namespaces"
	"hubcr.io/hubcr/internal/modules/organizations"
	"hubcr.io/hubcr/internal/modules/repositories"
	"hubcr.io/hubcr/internal/modules/security"
	"hubcr.io/hubcr/internal/platform/httpapi"
	"hubcr.io/hubcr/internal/platform/httpapi/authhandler"
)

type Authenticator interface {
	Authenticate(context.Context, string) (auth.User, error)
}

// NamespaceAccessSource resolves the actor's relationship to a namespace by name.
// repositories.Service satisfies this via ResolveNamespaceAccess.
type NamespaceAccessSource interface {
	ResolveNamespaceAccess(context.Context, string, string) (repositories.NamespaceAccess, error)
}

// TrustService is the subset of *security.TrustService used by this handler.
type TrustService interface {
	CurrentPolicy(context.Context, string, string) (security.TrustPolicy, error)
	CreatePolicy(context.Context, string, string, []security.PublicKeyTrust, []security.KeylessIdentity) (security.TrustPolicy, error)
}

type Handler struct {
	authenticator Authenticator
	access        NamespaceAccessSource
	trust         TrustService
}

// New constructs the trust-policy management handler. The trust service must be the
// managing variant (security.NewManagingTrustService) so authorization is enforced.
func New(authenticator Authenticator, access NamespaceAccessSource, trust TrustService) (*Handler, error) {
	if authenticator == nil || access == nil || trust == nil {
		return nil, errors.New("trust policy handler dependencies must be configured")
	}
	return &Handler{authenticator: authenticator, access: access, trust: trust}, nil
}

func RegisterRoutes(router *httpapi.Router, handler *Handler) {
	router.Handle(http.MethodGet, "/api/v1/namespaces/{namespace}/trust-policy", handler.current)
	router.Handle(http.MethodPost, "/api/v1/namespaces/{namespace}/trust-policy", handler.create)
}

type publicKeyRequest struct {
	Name         string `json:"name"`
	PublicKeyPEM string `json:"public_key_pem"`
	Fingerprint  string `json:"fingerprint"`
}

type keylessIdentityRequest struct {
	Issuer  string `json:"issuer"`
	Subject string `json:"subject"`
}

type createPolicyRequest struct {
	PublicKeys        []publicKeyRequest       `json:"public_keys"`
	KeylessIdentities []keylessIdentityRequest `json:"keyless_identities"`
}

type publicKeyResponse struct {
	Name         string `json:"name"`
	Fingerprint  string `json:"fingerprint"`
	PublicKeyPEM string `json:"public_key_pem"`
}

type keylessIdentityResponse struct {
	Issuer  string `json:"issuer"`
	Subject string `json:"subject"`
}

type trustPolicyResponse struct {
	ID                string                    `json:"id"`
	Version           int64                     `json:"version"`
	PublicKeys        []publicKeyResponse       `json:"public_keys"`
	KeylessIdentities []keylessIdentityResponse `json:"keyless_identities"`
	CreatedByUserID   string                    `json:"created_by_user_id"`
	CreatedAt         string                    `json:"created_at"`
}

func (h *Handler) current(w http.ResponseWriter, request *http.Request) error {
	user, err := h.currentUser(request)
	if err != nil {
		return err
	}
	namespaceName, apiError := namespacePath(request)
	if apiError != nil {
		return apiError
	}
	policy, err := h.trust.CurrentPolicy(request.Context(), namespaceName, string(user.ID))
	if err != nil {
		return mapError(err)
	}
	httpapi.WriteJSON(w, http.StatusOK, mapPolicy(policy))
	return nil
}

func (h *Handler) create(w http.ResponseWriter, request *http.Request) error {
	if err := rejectCrossSite(request); err != nil {
		return err
	}
	user, err := h.currentUser(request)
	if err != nil {
		return err
	}
	namespaceName, apiError := namespacePath(request)
	if apiError != nil {
		return apiError
	}
	var input createPolicyRequest
	if apiError := httpapi.DecodeJSON(w, request, &input); apiError != nil {
		return apiError
	}
	keys, identities, validation := validateCreate(input)
	if validation != nil {
		return validation
	}
	policy, err := h.trust.CreatePolicy(request.Context(), namespaceName, string(user.ID), keys, identities)
	if err != nil {
		return mapError(err)
	}
	httpapi.WriteJSON(w, http.StatusCreated, mapPolicy(policy))
	return nil
}

func (h *Handler) currentUser(request *http.Request) (auth.User, error) {
	cookie, err := request.Cookie(authhandler.SessionCookieName)
	if err != nil || cookie.Value == "" {
		return auth.User{}, httpapi.AuthenticationFailed()
	}
	user, err := h.authenticator.Authenticate(request.Context(), cookie.Value)
	if err != nil {
		if errors.Is(err, auth.ErrUnauthenticated) {
			return auth.User{}, httpapi.AuthenticationFailed()
		}
		return auth.User{}, err
	}
	return user, nil
}

func namespacePath(request *http.Request) (string, *httpapi.Error) {
	name := request.PathValue("namespace")
	if _, err := namespaces.NormalizeName(name); err != nil {
		return "", httpapi.InvalidRequest("namespace name is invalid")
	}
	return name, nil
}

func rejectCrossSite(request *http.Request) *httpapi.Error {
	if request.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return httpapi.InvalidRequest("cross-site request rejected")
	}
	return nil
}

// validateCreate normalizes the request body into validated domain values. Public keys
// must be supplied as canonical PEM with an exact fingerprint, matching the immutable
// PublicKeyTrust contract; the request fingerprint is checked against the recomputed one
// so clients cannot assert a false identity. Keyless identities use exact issuer/subject.
func validateCreate(input createPolicyRequest) ([]security.PublicKeyTrust, []security.KeylessIdentity, *httpapi.Error) {
	total := len(input.PublicKeys) + len(input.KeylessIdentities)
	if total < 1 {
		return nil, nil, httpapi.ValidationFailed(httpapi.FieldError{
			Field: "public_keys", Message: "at least one trust subject is required",
		})
	}
	if total > security.MaxTrustPolicySubjects {
		return nil, nil, httpapi.ValidationFailed(httpapi.FieldError{
			Field: "public_keys", Message: "too many trust subjects",
		})
	}
	fields := make([]httpapi.FieldError, 0)
	keys := make([]security.PublicKeyTrust, 0, len(input.PublicKeys))
	for i, item := range input.PublicKeys {
		key, err := security.NewPublicKeyTrust(item.Name, []byte(item.PublicKeyPEM))
		if err != nil {
			fields = append(fields, httpapi.FieldError{
				Field:   "public_keys[" + indexLabel(i) + "]",
				Message: "must be a canonical PUBLIC KEY PEM with a matching sha256 fingerprint",
			})
			continue
		}
		if item.Fingerprint != "" && item.Fingerprint != key.Fingerprint {
			fields = append(fields, httpapi.FieldError{
				Field:   "public_keys[" + indexLabel(i) + "].fingerprint",
				Message: "fingerprint does not match the supplied public key",
			})
			continue
		}
		keys = append(keys, key)
	}
	identities := make([]security.KeylessIdentity, 0, len(input.KeylessIdentities))
	for i, item := range input.KeylessIdentities {
		identity, err := security.NewKeylessIdentity(item.Issuer, item.Subject)
		if err != nil {
			fields = append(fields, httpapi.FieldError{
				Field:   "keyless_identities[" + indexLabel(i) + "]",
				Message: "must be an exact OIDC issuer and subject without wildcards",
			})
			continue
		}
		identities = append(identities, identity)
	}
	if len(fields) > 0 {
		return nil, nil, httpapi.ValidationFailed(fields...)
	}
	return keys, identities, nil
}

func indexLabel(i int) string {
	if i <= 0 {
		return "0"
	}
	digits := []byte{}
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return string(digits)
}

func mapPolicy(policy security.TrustPolicy) trustPolicyResponse {
	keys := make([]publicKeyResponse, 0, len(policy.PublicKeys))
	for _, key := range policy.PublicKeys {
		keys = append(keys, publicKeyResponse{
			Name: key.Name, Fingerprint: key.Fingerprint, PublicKeyPEM: key.PublicKeyPEM,
		})
	}
	identities := make([]keylessIdentityResponse, 0, len(policy.KeylessIdentities))
	for _, identity := range policy.KeylessIdentities {
		identities = append(identities, keylessIdentityResponse{Issuer: identity.Issuer, Subject: identity.Subject})
	}
	return trustPolicyResponse{
		ID: policy.ID, Version: policy.Version, PublicKeys: keys, KeylessIdentities: identities,
		CreatedByUserID: policy.CreatedByUserID, CreatedAt: httpapi.FormatTime(policy.CreatedAt),
	}
}

func mapError(err error) error {
	switch {
	case errors.Is(err, security.ErrForbidden):
		// Read denial for a non-member must not leak namespace existence; create denial
		// for a non-owner is also mapped to 404 to keep the resource hidden.
		return httpapi.NotFound()
	case errors.Is(err, security.ErrNotFound):
		return httpapi.NotFound()
	case errors.Is(err, security.ErrInvalid), errors.Is(err, repositories.ErrInvalidName):
		return httpapi.InvalidRequest("trust policy request is invalid")
	case errors.Is(err, security.ErrConflict):
		return httpapi.Conflict("trust policy conflicts with persisted state")
	default:
		return err
	}
}

// ResolveAccess adapts repositories.Service into a security.NamespaceAccessResolver,
// translating the repository module's access value into the security module's type. This
// keeps the security module free of a repositories import.
func ResolveAccess(source NamespaceAccessSource) security.NamespaceAccessResolver {
	return func(ctx context.Context, namespaceName, actorUserID string) (security.NamespaceAccess, error) {
		access, err := source.ResolveNamespaceAccess(ctx, namespaceName, actorUserID)
		if err != nil {
			if errors.Is(err, repositories.ErrNotFound) {
				return security.NamespaceAccess{}, security.ErrNotFound
			}
			return security.NamespaceAccess{}, err
		}
		return security.NamespaceAccess{
			NamespaceID:      access.NamespaceID,
			Kind:             security.NamespaceKind(access.Kind),
			IsPersonalOwner:  access.IsPersonalOwner,
			OrganizationRole: organizations.Role(access.OrganizationRole),
		}, nil
	}
}
