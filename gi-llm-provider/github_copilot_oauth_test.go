package gillmprovider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestParseAvailableGitHubCopilotModelIDs(t *testing.T) {
	raw := []byte(`{
		"data": [
			{
				"id": "gpt-4.1",
				"model_picker_enabled": true,
				"capabilities": {"supports": {"tool_calls": true}}
			},
			{
				"id": "disabled-by-policy",
				"model_picker_enabled": true,
				"policy": {"state": "disabled"}
			},
			{
				"id": "disabled-by-capability",
				"model_picker_enabled": true,
				"capabilities": {"supports": {"tool_calls": false}}
			},
			{
				"id": "disabled-in-picker",
				"model_picker_enabled": false
			},
			{
				"id": "missing-capabilities",
				"model_picker_enabled": true
			},
			{
				"id": "non-object-nested-fields",
				"model_picker_enabled": true,
				"policy": "unknown",
				"capabilities": "unknown"
			},
			"not-an-object"
		]
	}`)

	got, err := parseAvailableGitHubCopilotModelIDs(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"gpt-4.1",
		"missing-capabilities",
		"non-object-nested-fields",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("model IDs = %#v, want %#v", got, want)
	}

	for _, invalid := range []string{
		`{}`,
		`{"data": null}`,
		`{"data": {}}`,
		`{"data":`,
	} {
		if _, err := parseAvailableGitHubCopilotModelIDs(
			[]byte(invalid),
		); err == nil {
			t.Fatalf("parseAvailableGitHubCopilotModelIDs(%q) succeeded", invalid)
		}
	}
}

func TestGitHubCopilotOAuthRefreshesAndLoadsAccountModels(t *testing.T) {
	const (
		refreshToken = "ghu_refresh_token"
		accessToken  = "tid=test;proxy-ep=proxy.enterprise.copilot.test;"
	)
	var requested []string
	client := githubCopilotOAuthDoerFunc(func(
		request *http.Request,
	) (*http.Response, error) {
		requested = append(requested, request.URL.String())
		switch request.URL.String() {
		case "https://api.enterprise.example/copilot_internal/v2/token":
			if got := request.Header.Get("Authorization"); got !=
				"Bearer "+refreshToken {
				return nil, fmt.Errorf("token authorization = %q", got)
			}
			return githubCopilotJSONResponse(
				http.StatusOK,
				`{"token":"`+accessToken+`","expires_at":2000000000}`,
			), nil
		case "https://api.enterprise.copilot.test/models":
			if got := request.Header.Get("Authorization"); got !=
				"Bearer "+accessToken {
				return nil, fmt.Errorf("models authorization = %q", got)
			}
			if got := request.Header.Get("X-GitHub-Api-Version"); got !=
				githubCopilotAPIVersion {
				return nil, fmt.Errorf("models API version = %q", got)
			}
			return githubCopilotJSONResponse(
				http.StatusOK,
				`{"data":[
					{"id":"gpt-4.1","model_picker_enabled":true},
					{"id":"disabled","model_picker_enabled":false}
				]}`,
			), nil
		default:
			return nil, fmt.Errorf("unexpected URL %s", request.URL)
		}
	})
	auth := NewGitHubCopilotOAuth(GitHubCopilotOAuthOptions{
		Client:              client,
		RequestTimeout:      time.Second,
		ModelCatalogTimeout: time.Second,
	})
	previous := Credential{
		Type:          CredentialTypeOAuth,
		Access:        "expired",
		Refresh:       refreshToken,
		EnterpriseURL: "https://Enterprise.Example/some/path",
		Env:           ProviderEnv{"scope": "test"},
		Metadata:      map[string]any{"preserved": true},
	}

	refreshed, err := auth.Refresh(context.Background(), previous)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.Type != CredentialTypeOAuth ||
		refreshed.Access != accessToken ||
		refreshed.Refresh != refreshToken ||
		refreshed.Expires != 2000000000*1000-
			githubCopilotRefreshSkew.Milliseconds() ||
		refreshed.EnterpriseURL != "enterprise.example" {
		t.Fatalf("refreshed credential = %#v", refreshed)
	}
	if refreshed.Env["scope"] != "test" ||
		refreshed.Metadata["preserved"] != true {
		t.Fatalf("refreshed metadata = %#v, env = %#v", refreshed.Metadata, refreshed.Env)
	}
	if got, ok := refreshed.Metadata["availableModelIds"].([]string); !ok || !reflect.DeepEqual(got, []string{"gpt-4.1"}) {
		t.Fatalf("available model IDs = %#v", refreshed.Metadata["availableModelIds"])
	}
	if len(requested) != 2 {
		t.Fatalf("requests = %#v", requested)
	}

	modelAuth, err := auth.ToAuth(context.Background(), refreshed)
	if err != nil {
		t.Fatal(err)
	}
	if modelAuth.APIKey != accessToken ||
		modelAuth.BaseURL != "https://api.enterprise.copilot.test" {
		t.Fatalf("model auth = %#v", modelAuth)
	}
}

func TestGitHubCopilotOAuthLoginDataFlow(t *testing.T) {
	const (
		refreshToken = "ghu_device_access"
		accessToken  = "tid=test;proxy-ep=proxy.ghe.example.test;"
	)
	var (
		requestMu    sync.Mutex
		requests     []string
		enabledModel = map[string]bool{}
	)
	client := githubCopilotOAuthDoerFunc(func(
		request *http.Request,
	) (*http.Response, error) {
		requestMu.Lock()
		requests = append(requests, request.Method+" "+request.URL.String())
		requestMu.Unlock()

		switch request.URL.Path {
		case "/login/device/code":
			if err := request.ParseForm(); err != nil {
				return nil, err
			}
			if request.Form.Get("client_id") != githubCopilotOAuthClientID ||
				request.Form.Get("scope") != "read:user" {
				return nil, fmt.Errorf("device form = %#v", request.Form)
			}
			return githubCopilotJSONResponse(
				http.StatusOK,
				`{
					"device_code":"device-code",
					"user_code":"ABCD-EFGH",
					"verification_uri":"https://ghe.example.test/login/device",
					"interval":7,
					"expires_in":900
				}`,
			), nil
		case "/login/oauth/access_token":
			if err := request.ParseForm(); err != nil {
				return nil, err
			}
			if request.Form.Get("device_code") != "device-code" ||
				request.Form.Get("grant_type") != githubCopilotDeviceGrant {
				return nil, fmt.Errorf("token form = %#v", request.Form)
			}
			return githubCopilotJSONResponse(
				http.StatusOK,
				`{"access_token":"`+refreshToken+`"}`,
			), nil
		case "/copilot_internal/v2/token":
			if request.Header.Get("Authorization") !=
				"Bearer "+refreshToken {
				return nil, fmt.Errorf(
					"refresh authorization = %q",
					request.Header.Get("Authorization"),
				)
			}
			return githubCopilotJSONResponse(
				http.StatusOK,
				`{"token":"`+accessToken+`","expires_at":2000000000}`,
			), nil
		case "/models":
			if request.Header.Get("X-GitHub-Api-Version") !=
				githubCopilotAPIVersion {
				return nil, fmt.Errorf(
					"catalog API version = %q",
					request.Header.Get("X-GitHub-Api-Version"),
				)
			}
			return githubCopilotJSONResponse(
				http.StatusOK,
				`{"data":[
					{"id":"model-one","model_picker_enabled":true},
					{"id":"model-two","model_picker_enabled":true}
				]}`,
			), nil
		default:
			if strings.HasPrefix(request.URL.Path, "/models/") &&
				strings.HasSuffix(request.URL.Path, "/policy") {
				parts := strings.Split(request.URL.Path, "/")
				if len(parts) != 4 {
					return nil, fmt.Errorf(
						"unexpected policy path %q",
						request.URL.Path,
					)
				}
				requestMu.Lock()
				enabledModel[parts[2]] = true
				requestMu.Unlock()
				return githubCopilotJSONResponse(http.StatusOK, `{}`), nil
			}
			return nil, fmt.Errorf("unexpected URL %s", request.URL)
		}
	})
	var pollOptions OAuthDeviceCodePollOptions[string]
	auth := newGitHubCopilotOAuth(
		GitHubCopilotOAuthOptions{
			Client:              client,
			RequestTimeout:      time.Second,
			ModelCatalogTimeout: time.Second,
		},
		githubCopilotOAuthRuntime{
			pollDeviceCode: func(
				ctx context.Context,
				options OAuthDeviceCodePollOptions[string],
			) (string, error) {
				pollOptions = options
				result, err := options.Poll(ctx)
				if err != nil {
					return "", err
				}
				if result.Status != OAuthDeviceCodeComplete {
					return "", fmt.Errorf("poll result = %#v", result)
				}
				return result.Value, nil
			},
			models: func() []Model {
				return []Model{{ID: "model-one"}, {ID: "model-two"}}
			},
		},
	)
	interaction := &githubCopilotTestInteraction{
		answer: "https://GHE.Example.Test/company",
	}

	credential, err := auth.Login(context.Background(), interaction)
	if err != nil {
		t.Fatal(err)
	}
	if credential.Access != accessToken ||
		credential.Refresh != refreshToken ||
		credential.EnterpriseURL != "ghe.example.test" {
		t.Fatalf("credential = %#v", credential)
	}
	if !pollOptions.WaitBeforeFirstPoll ||
		pollOptions.IntervalSeconds != 7 ||
		pollOptions.ExpiresInSeconds != 900 {
		t.Fatalf("poll options = %#v", pollOptions)
	}
	if len(interaction.events) != 2 ||
		interaction.events[0].Type != AuthEventDeviceCode ||
		interaction.events[0].UserCode != "ABCD-EFGH" ||
		interaction.events[0].VerificationURI !=
			"https://ghe.example.test/login/device" ||
		interaction.events[1].Type != AuthEventProgress ||
		interaction.events[1].Message != "Enabling models..." {
		t.Fatalf("events = %#v", interaction.events)
	}
	requestMu.Lock()
	defer requestMu.Unlock()
	if !enabledModel["model-one"] || !enabledModel["model-two"] {
		t.Fatalf("enabled models = %#v; requests = %#v", enabledModel, requests)
	}
}

func TestGitHubCopilotOAuthRejectsUntrustedVerificationURI(t *testing.T) {
	client := githubCopilotOAuthDoerFunc(func(
		request *http.Request,
	) (*http.Response, error) {
		return githubCopilotJSONResponse(
			http.StatusOK,
			`{
				"device_code":"device-code",
				"user_code":"ABCD-EFGH",
				"verification_uri":"file:///tmp/not-a-browser-url",
				"interval":1,
				"expires_in":900
			}`,
		), nil
	})
	config := githubCopilotOAuthConfig{
		client:              client,
		requestTimeout:      time.Second,
		modelCatalogTimeout: time.Second,
	}

	_, err := requestGitHubCopilotDeviceCode(
		context.Background(),
		config,
		githubCopilotDefaultDomain,
	)
	if err == nil || !strings.Contains(err.Error(), "Untrusted verification_uri") {
		t.Fatalf("verification URI error = %v", err)
	}
}

func TestGitHubCopilotBuiltinOAuthSupportsApplicationOverride(t *testing.T) {
	const providerID = "github-copilot"
	previousLoader := getOAuthAuthLoader(providerID)
	UnregisterOAuthAuthLoader(providerID)
	t.Cleanup(func() {
		RegisterOAuthAuthLoader(providerID, previousLoader)
	})

	provider, err := NewBuiltinProvider(providerID)
	if err != nil {
		t.Fatal(err)
	}
	builtin, err := provider.Auth.OAuth.ToAuth(
		context.Background(),
		Credential{
			Type:   CredentialTypeOAuth,
			Access: "tid=test;proxy-ep=proxy.individual.githubcopilot.com;",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if builtin.BaseURL != "https://api.individual.githubcopilot.com" {
		t.Fatalf("built-in auth = %#v", builtin)
	}

	RegisterOAuthAuthLoader(
		providerID,
		func(context.Context) (*OAuthAuth, error) {
			return &OAuthAuth{
				ToAuth: func(
					context.Context,
					Credential,
				) (ModelAuth, error) {
					return ModelAuth{APIKey: "application-override"}, nil
				},
			}, nil
		},
	)
	overriddenProvider, err := NewBuiltinProvider(providerID)
	if err != nil {
		t.Fatal(err)
	}
	overridden, err := overriddenProvider.Auth.OAuth.ToAuth(
		context.Background(),
		Credential{Type: CredentialTypeOAuth, Access: "unused"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if overridden.APIKey != "application-override" {
		t.Fatalf("overridden auth = %#v", overridden)
	}
}

type githubCopilotOAuthDoerFunc func(
	*http.Request,
) (*http.Response, error)

func (do githubCopilotOAuthDoerFunc) Do(
	request *http.Request,
) (*http.Response, error) {
	return do(request)
}

type githubCopilotTestInteraction struct {
	answer string
	prompt AuthPrompt
	events []AuthEvent
}

func (i *githubCopilotTestInteraction) Prompt(
	_ context.Context,
	prompt AuthPrompt,
) (string, error) {
	i.prompt = prompt
	return i.answer, nil
}

func (i *githubCopilotTestInteraction) Notify(event AuthEvent) {
	i.events = append(i.events, event)
}

func githubCopilotJSONResponse(
	statusCode int,
	body string,
) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Status:     fmt.Sprintf("%d %s", statusCode, http.StatusText(statusCode)),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
