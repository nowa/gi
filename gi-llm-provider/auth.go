package gillmprovider

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ModelAuth is the complete provider-scoped authentication result for one
// request. Provider-specific configuration that cannot be expressed here
// belongs on the provider itself.
type ModelAuth struct {
	APIKey         string
	Headers        map[string]string
	HeaderRemovals []string
	BaseURL        string
}

// AuthContext isolates ambient process state so auth resolution remains
// deterministic in tests and reusable by non-CLI applications.
type AuthContext interface {
	Env(ctx context.Context, name string) (value string, ok bool, err error)
	FileExists(ctx context.Context, path string) (bool, error)
}

// AuthContextFuncs adapts functions to AuthContext. Missing functions use the
// same behavior as ProcessAuthContext.
type AuthContextFuncs struct {
	EnvFunc        func(ctx context.Context, name string) (value string, ok bool, err error)
	FileExistsFunc func(ctx context.Context, path string) (bool, error)
}

func (f AuthContextFuncs) Env(ctx context.Context, name string) (string, bool, error) {
	if f.EnvFunc != nil {
		return f.EnvFunc(contextOrBackground(ctx), name)
	}
	return (ProcessAuthContext{}).Env(ctx, name)
}

func (f AuthContextFuncs) FileExists(ctx context.Context, path string) (bool, error) {
	if f.FileExistsFunc != nil {
		return f.FileExistsFunc(contextOrBackground(ctx), path)
	}
	return (ProcessAuthContext{}).FileExists(ctx, path)
}

// ProcessAuthContext resolves environment variables and local file presence.
// Its zero value is ready for use.
type ProcessAuthContext struct{}

func (ProcessAuthContext) Env(ctx context.Context, name string) (string, bool, error) {
	if err := contextError(ctx); err != nil {
		return "", false, err
	}
	value, ok := os.LookupEnv(name)
	if !ok || strings.TrimSpace(value) == "" {
		return "", false, nil
	}
	return value, true, nil
}

func (ProcessAuthContext) FileExists(ctx context.Context, path string) (bool, error) {
	if err := contextError(ctx); err != nil {
		return false, err
	}
	resolved, err := expandHomePath(path)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(resolved)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, os.ErrNotExist):
		return false, nil
	default:
		return false, err
	}
}

// DefaultProviderAuthContext returns the process-backed auth context.
func DefaultProviderAuthContext() AuthContext {
	return ProcessAuthContext{}
}

func expandHomePath(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") && !strings.HasPrefix(path, `~\`) {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if path == "~" {
		return home, nil
	}
	return filepath.Join(home, path[2:]), nil
}

// AuthResult contains resolved request auth plus status-facing provenance.
type AuthResult struct {
	Auth   ModelAuth
	Env    ProviderEnv
	Source string
}

// AuthCheck describes auth availability without resolving request secrets.
type AuthCheck struct {
	Source string
	Type   CredentialType
}

// AuthPromptType identifies an interactive login input shape.
type AuthPromptType string

const (
	// AuthPromptText requests visible free-form text.
	AuthPromptText AuthPromptType = "text"
	// AuthPromptSecret requests a secret value.
	AuthPromptSecret AuthPromptType = "secret"
	// AuthPromptSelect requests one option identifier.
	AuthPromptSelect AuthPromptType = "select"
	// AuthPromptManualCode requests an OAuth callback code.
	AuthPromptManualCode AuthPromptType = "manual_code"
)

// AuthPromptOption is one selectable login response.
type AuthPromptOption struct {
	ID          string
	Label       string
	Description string
}

// AuthPrompt describes one interactive login request.
type AuthPrompt struct {
	Type        AuthPromptType
	Message     string
	Placeholder string
	Options     []AuthPromptOption
}

// AuthInfoLink is a user-facing link attached to an auth event.
type AuthInfoLink struct {
	URL   string
	Label string
}

// AuthEventType identifies an out-of-band login notification.
type AuthEventType string

const (
	// AuthEventInfo reports general login information.
	AuthEventInfo AuthEventType = "info"
	// AuthEventURL asks the user to open an authorization URL.
	AuthEventURL AuthEventType = "auth_url"
	// AuthEventDeviceCode reports a device authorization challenge.
	AuthEventDeviceCode AuthEventType = "device_code"
	// AuthEventProgress reports non-terminal login progress.
	AuthEventProgress AuthEventType = "progress"
)

// AuthEvent describes an out-of-band login notification.
type AuthEvent struct {
	Type             AuthEventType
	Message          string
	Links            []AuthInfoLink
	URL              string
	Instructions     string
	UserCode         string
	VerificationURI  string
	IntervalSeconds  int
	ExpiresInSeconds int
}

// AuthInteraction is implemented by the application so provider login flows
// remain independent of any terminal or UI package.
type AuthInteraction interface {
	Prompt(ctx context.Context, prompt AuthPrompt) (string, error)
	Notify(event AuthEvent)
}

// APIKeyResolveInput supplies deterministic context and an optional stored
// credential to an API-key resolver.
type APIKeyResolveInput struct {
	Context    AuthContext
	Credential *Credential
}

// APIKeyCheckInput supplies deterministic context for a side-effect-free auth
// availability check.
type APIKeyCheckInput struct {
	Context    AuthContext
	Credential *Credential
}

// APIKeyAuth defines provider-specific API-key login and resolution.
type APIKeyAuth struct {
	Name    string
	Login   func(ctx context.Context, interaction AuthInteraction) (Credential, error)
	Check   func(ctx context.Context, input APIKeyCheckInput) (*AuthCheck, error)
	Resolve func(ctx context.Context, input APIKeyResolveInput) (*AuthResult, error)
}

// OAuthAuth separates token refresh from side-effect-free request auth
// derivation so refresh can run within a credential-store transaction.
type OAuthAuth struct {
	Name       string
	LoginLabel string
	Login      func(ctx context.Context, interaction AuthInteraction) (Credential, error)
	Refresh    func(ctx context.Context, credential Credential) (Credential, error)
	ToAuth     func(ctx context.Context, credential Credential) (ModelAuth, error)
}

// ProviderAuth groups the authentication modes supported by a provider.
type ProviderAuth struct {
	APIKey *APIKeyAuth
	OAuth  *OAuthAuth
}

// AuthResolutionOverrides contains request-scoped key and environment values.
type AuthResolutionOverrides struct {
	// APIKey is a pointer so callers can distinguish no override from an
	// explicit empty key.
	APIKey *string
	Env    ProviderEnv
}

// ModelsErrorCode is a stable machine-readable models subsystem error class.
type ModelsErrorCode string

const (
	// ModelsErrorModelSource reports model source loading failures.
	ModelsErrorModelSource ModelsErrorCode = "model_source"
	// ModelsErrorModelValidation reports invalid model definitions.
	ModelsErrorModelValidation ModelsErrorCode = "model_validation"
	// ModelsErrorProvider reports provider registration or lookup failures.
	ModelsErrorProvider ModelsErrorCode = "provider"
	// ModelsErrorStream reports request streaming failures.
	ModelsErrorStream ModelsErrorCode = "stream"
	// ModelsErrorAuth reports API-key or credential-store failures.
	ModelsErrorAuth ModelsErrorCode = "auth"
	// ModelsErrorOAuth reports OAuth refresh or derivation failures.
	ModelsErrorOAuth ModelsErrorCode = "oauth"
)

// ModelsError preserves a stable machine-readable category while retaining
// the underlying cause for errors.Is and errors.As.
type ModelsError struct {
	Code ModelsErrorCode
	Msg  string
	Err  error
}

func (e *ModelsError) Error() string {
	if e == nil {
		return ""
	}
	message := strings.TrimSpace(e.Msg)
	if e.Err == nil {
		return message
	}
	detail := strings.TrimSpace(e.Err.Error())
	if detail == "" || strings.Contains(message, detail) {
		return message
	}
	if message == "" {
		return detail
	}
	return message + ": " + detail
}

func (e *ModelsError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func newModelsError(code ModelsErrorCode, message string, cause error) error {
	return &ModelsError{Code: code, Msg: message, Err: cause}
}

// ResolveProviderAuth resolves the effective authentication for a provider.
// A stored credential owns the provider: ambient sources are consulted only
// when no credential exists.
func ResolveProviderAuth(
	ctx context.Context,
	providerID string,
	auth ProviderAuth,
	credentials CredentialStore,
	authContext AuthContext,
	overrides AuthResolutionOverrides,
) (*AuthResult, error) {
	ctx = contextOrBackground(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if credentials == nil {
		credentials = NewInMemoryCredentialStore()
	}
	if authContext == nil {
		authContext = DefaultProviderAuthContext()
	}
	requestContext := overlayProviderAuthContext(authContext, overrides.Env)

	if overrides.APIKey != nil && auth.APIKey != nil {
		credential := Credential{
			Type: CredentialTypeAPIKey,
			Key:  *overrides.APIKey,
			Env:  cloneProviderEnv(overrides.Env),
		}
		return resolveAPIKeyAuth(ctx, requestContext, *auth.APIKey, providerID, &credential)
	}

	stored, exists, err := credentials.ReadCredential(ctx, providerID)
	if err != nil {
		return nil, newModelsError(
			ModelsErrorAuth,
			fmt.Sprintf("Credential store read failed for %s", providerID),
			err,
		)
	}
	if exists {
		switch {
		case stored.Type == CredentialTypeOAuth && auth.OAuth != nil:
			return resolveStoredOAuth(ctx, credentials, providerID, *auth.OAuth, stored)
		case stored.Type == CredentialTypeAPIKey && auth.APIKey != nil:
			if len(overrides.Env) > 0 {
				stored.Env = mergeProviderEnv(stored.Env, overrides.Env)
			}
			return resolveAPIKeyAuth(ctx, requestContext, *auth.APIKey, providerID, &stored)
		default:
			return nil, nil
		}
	}

	if auth.APIKey == nil {
		return nil, nil
	}
	return resolveAPIKeyAuth(ctx, requestContext, *auth.APIKey, providerID, nil)
}

func resolveStoredOAuth(
	ctx context.Context,
	credentials CredentialStore,
	providerID string,
	oauth OAuthAuth,
	stored Credential,
) (*AuthResult, error) {
	credential := stored.Clone()
	if time.Now().UnixMilli() >= credential.Expires {
		post, exists, err := credentials.ModifyCredential(
			ctx,
			providerID,
			func(ctx context.Context, current Credential, exists bool) (Credential, bool, error) {
				if !exists || current.Type != CredentialTypeOAuth {
					return Credential{}, false, nil
				}
				if time.Now().UnixMilli() < current.Expires {
					return Credential{}, false, nil
				}
				if oauth.Refresh == nil {
					return Credential{}, false, newModelsError(
						ModelsErrorOAuth,
						fmt.Sprintf("OAuth refresh failed for %s", providerID),
						errors.New("OAuth refresh is not configured"),
					)
				}
				refreshed, err := oauth.Refresh(ctx, current.Clone())
				if err != nil {
					return Credential{}, false, newModelsError(
						ModelsErrorOAuth,
						fmt.Sprintf("OAuth refresh failed for %s", providerID),
						err,
					)
				}
				if refreshed.Type != CredentialTypeOAuth {
					return Credential{}, false, newModelsError(
						ModelsErrorOAuth,
						fmt.Sprintf("OAuth refresh failed for %s", providerID),
						fmt.Errorf("refresh returned credential type %q", refreshed.Type),
					)
				}
				return refreshed, true, nil
			},
		)
		if err != nil {
			var modelsErr *ModelsError
			if errors.As(err, &modelsErr) {
				return nil, err
			}
			return nil, newModelsError(
				ModelsErrorAuth,
				fmt.Sprintf("Credential store modify failed for %s", providerID),
				err,
			)
		}
		if !exists || post.Type != CredentialTypeOAuth {
			return nil, nil
		}
		credential = post
	}

	if oauth.ToAuth == nil {
		return nil, newModelsError(
			ModelsErrorOAuth,
			fmt.Sprintf("OAuth auth derivation failed for %s", providerID),
			errors.New("OAuth auth derivation is not configured"),
		)
	}
	modelAuth, err := oauth.ToAuth(ctx, credential.Clone())
	if err != nil {
		return nil, newModelsError(
			ModelsErrorOAuth,
			fmt.Sprintf("OAuth auth derivation failed for %s", providerID),
			err,
		)
	}
	return &AuthResult{Auth: cloneModelAuth(modelAuth), Source: "OAuth"}, nil
}

func resolveAPIKeyAuth(
	ctx context.Context,
	authContext AuthContext,
	apiKey APIKeyAuth,
	providerID string,
	credential *Credential,
) (*AuthResult, error) {
	if apiKey.Resolve == nil {
		return nil, nil
	}
	var inputCredential *Credential
	if credential != nil {
		cloned := credential.Clone()
		inputCredential = &cloned
	}
	result, err := apiKey.Resolve(ctx, APIKeyResolveInput{
		Context:    authContext,
		Credential: inputCredential,
	})
	if err != nil {
		return nil, newModelsError(
			ModelsErrorAuth,
			fmt.Sprintf("API key auth failed for provider %s", providerID),
			err,
		)
	}
	if result == nil {
		return nil, nil
	}
	cloned := cloneAuthResult(*result)
	return &cloned, nil
}

type overlaidAuthContext struct {
	base AuthContext
	env  ProviderEnv
}

func overlayProviderAuthContext(base AuthContext, env ProviderEnv) AuthContext {
	if len(env) == 0 {
		return base
	}
	return overlaidAuthContext{base: base, env: cloneProviderEnv(env)}
}

func (c overlaidAuthContext) Env(ctx context.Context, name string) (string, bool, error) {
	if value := c.env[name]; strings.TrimSpace(value) != "" {
		return value, true, nil
	}
	return c.base.Env(ctx, name)
}

func (c overlaidAuthContext) FileExists(ctx context.Context, path string) (bool, error) {
	return c.base.FileExists(ctx, path)
}

func mergeProviderEnv(base, overlay ProviderEnv) ProviderEnv {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	merged := cloneProviderEnv(base)
	if merged == nil {
		merged = ProviderEnv{}
	}
	for key, value := range overlay {
		merged[key] = value
	}
	return merged
}

func cloneModelAuth(auth ModelAuth) ModelAuth {
	cloned := auth
	if auth.Headers != nil {
		cloned.Headers = make(map[string]string, len(auth.Headers))
		for key, value := range auth.Headers {
			cloned.Headers[key] = value
		}
	}
	cloned.HeaderRemovals = append([]string(nil), auth.HeaderRemovals...)
	return cloned
}

func cloneAuthResult(result AuthResult) AuthResult {
	result.Auth = cloneModelAuth(result.Auth)
	result.Env = cloneProviderEnv(result.Env)
	return result
}
