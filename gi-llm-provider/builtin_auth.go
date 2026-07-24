package gillmprovider

import (
	"context"
	"errors"
	"fmt"
)

const vertexADCPath = "~/.config/gcloud/application_default_credentials.json"

func amazonBedrockAuth() *APIKeyAuth {
	return &APIKeyAuth{
		Name: "AWS credentials or bearer token",
		Login: func(ctx context.Context, interaction AuthInteraction) (Credential, error) {
			if interaction == nil {
				return Credential{}, errors.New("auth interaction is required")
			}
			ctx = contextOrBackground(ctx)
			method, err := interaction.Prompt(ctx, AuthPrompt{
				Type:    AuthPromptSelect,
				Message: "Select Amazon Bedrock authentication method:",
				Options: []AuthPromptOption{
					{ID: "bearer-token", Label: "Bearer token"},
					{ID: "aws-profile", Label: "AWS profile"},
					{ID: "credential-chain", Label: "Existing AWS credential chain"},
				},
			})
			if err != nil {
				return Credential{}, err
			}
			if method == "bearer-token" {
				key, err := interaction.Prompt(ctx, AuthPrompt{
					Type:    AuthPromptSecret,
					Message: "Enter Amazon Bedrock bearer token",
				})
				if err != nil {
					return Credential{}, err
				}
				return Credential{Type: CredentialTypeAPIKey, Key: key}, nil
			}
			interaction.Notify(AuthEvent{
				Type:    AuthEventInfo,
				Message: "Amazon Bedrock supports AWS profiles, IAM credentials, and role-based credentials.",
				Links: []AuthInfoLink{{
					Label: "AWS credential provider chain",
					URL:   "https://docs.aws.amazon.com/sdkref/latest/guide/standardized-credentials.html",
				}},
			})
			switch method {
			case "aws-profile":
				profile, err := interaction.Prompt(ctx, AuthPrompt{
					Type:    AuthPromptText,
					Message: "Enter AWS profile name",
				})
				if err != nil {
					return Credential{}, err
				}
				return Credential{
					Type: CredentialTypeAPIKey,
					Env:  ProviderEnv{"AWS_PROFILE": profile},
				}, nil
			case "credential-chain":
				_, err := interaction.Prompt(ctx, AuthPrompt{
					Type:    AuthPromptText,
					Message: "Configure AWS credentials, then press Enter to continue",
				})
				if err != nil {
					return Credential{}, err
				}
				return Credential{Type: CredentialTypeAPIKey}, nil
			default:
				return Credential{}, fmt.Errorf("unknown Amazon Bedrock auth method: %s", method)
			}
		},
		Resolve: func(ctx context.Context, input APIKeyResolveInput) (*AuthResult, error) {
			ctx = contextOrBackground(ctx)
			authContext := authContextOrDefault(input.Context)
			if input.Credential != nil && input.Credential.Key != "" {
				return &AuthResult{
					Auth:   ModelAuth{APIKey: input.Credential.Key},
					Env:    cloneProviderEnv(input.Credential.Env),
					Source: "stored credential",
				}, nil
			}
			if value, err := authEnv(ctx, authContext, "AWS_BEARER_TOKEN_BEDROCK"); err != nil {
				return nil, err
			} else if value != "" {
				return &AuthResult{Source: "AWS_BEARER_TOKEN_BEDROCK"}, nil
			}
			if input.Credential != nil && input.Credential.Env["AWS_PROFILE"] != "" {
				return &AuthResult{
					Env:    cloneProviderEnv(input.Credential.Env),
					Source: "stored credential",
				}, nil
			}
			if value, err := authEnv(ctx, authContext, "AWS_PROFILE"); err != nil {
				return nil, err
			} else if value != "" {
				return &AuthResult{Source: "AWS_PROFILE"}, nil
			}
			access, err := authEnv(ctx, authContext, "AWS_ACCESS_KEY_ID")
			if err != nil {
				return nil, err
			}
			secret, err := authEnv(ctx, authContext, "AWS_SECRET_ACCESS_KEY")
			if err != nil {
				return nil, err
			}
			if access != "" && secret != "" {
				return &AuthResult{Source: "AWS access keys"}, nil
			}
			for _, candidate := range []struct {
				name   string
				source string
			}{
				{"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "ECS task role"},
				{"AWS_CONTAINER_CREDENTIALS_FULL_URI", "ECS task role"},
				{"AWS_WEB_IDENTITY_TOKEN_FILE", "web identity token"},
			} {
				value, err := authEnv(ctx, authContext, candidate.name)
				if err != nil {
					return nil, err
				}
				if value != "" {
					return &AuthResult{Source: candidate.source}, nil
				}
			}
			return nil, nil
		},
	}
}

func cloudflareWorkersAIAuth() *APIKeyAuth {
	return &APIKeyAuth{
		Name: "Cloudflare API key",
		Login: func(ctx context.Context, interaction AuthInteraction) (Credential, error) {
			key, accountID, _, err := promptCloudflareCredential(ctx, interaction, false)
			if err != nil {
				return Credential{}, err
			}
			return Credential{
				Type: CredentialTypeAPIKey,
				Key:  key,
				Env:  ProviderEnv{"CLOUDFLARE_ACCOUNT_ID": accountID},
			}, nil
		},
		Resolve: func(ctx context.Context, input APIKeyResolveInput) (*AuthResult, error) {
			resolved, err := resolveCloudflareAuth(ctx, input, false)
			if err != nil || resolved == nil {
				return nil, err
			}
			return &AuthResult{
				Auth:   ModelAuth{APIKey: resolved.apiKey},
				Env:    resolved.env,
				Source: resolved.source,
			}, nil
		},
	}
}

func cloudflareAIGatewayAuth() *APIKeyAuth {
	return &APIKeyAuth{
		Name: "Cloudflare API key",
		Login: func(ctx context.Context, interaction AuthInteraction) (Credential, error) {
			key, accountID, gatewayID, err := promptCloudflareCredential(ctx, interaction, true)
			if err != nil {
				return Credential{}, err
			}
			return Credential{
				Type: CredentialTypeAPIKey,
				Key:  key,
				Env: ProviderEnv{
					"CLOUDFLARE_ACCOUNT_ID": accountID,
					"CLOUDFLARE_GATEWAY_ID": gatewayID,
				},
			}, nil
		},
		Resolve: func(ctx context.Context, input APIKeyResolveInput) (*AuthResult, error) {
			resolved, err := resolveCloudflareAuth(ctx, input, true)
			if err != nil || resolved == nil {
				return nil, err
			}
			return &AuthResult{
				Auth: ModelAuth{
					Headers: map[string]string{
						"cf-aig-authorization": "Bearer " + resolved.apiKey,
					},
					HeaderRemovals: []string{"Authorization", "x-api-key"},
				},
				Env:    resolved.env,
				Source: resolved.source,
			}, nil
		},
	}
}

type cloudflareAuthResult struct {
	apiKey string
	env    ProviderEnv
	source string
}

func resolveCloudflareAuth(
	ctx context.Context,
	input APIKeyResolveInput,
	gateway bool,
) (*cloudflareAuthResult, error) {
	ctx = contextOrBackground(ctx)
	authContext := authContextOrDefault(input.Context)
	apiKey, err := credentialOrAmbientEnv(ctx, authContext, input.Credential, "CLOUDFLARE_API_KEY")
	if err != nil {
		return nil, err
	}
	accountID, err := credentialOrAmbientEnv(ctx, authContext, input.Credential, "CLOUDFLARE_ACCOUNT_ID")
	if err != nil {
		return nil, err
	}
	gatewayID := ""
	if gateway {
		gatewayID, err = credentialOrAmbientEnv(
			ctx,
			authContext,
			input.Credential,
			"CLOUDFLARE_GATEWAY_ID",
		)
		if err != nil {
			return nil, err
		}
	}
	if apiKey == "" || accountID == "" || gateway && gatewayID == "" {
		return nil, nil
	}
	env := ProviderEnv{"CLOUDFLARE_ACCOUNT_ID": accountID}
	if gatewayID != "" {
		env["CLOUDFLARE_GATEWAY_ID"] = gatewayID
	}
	source := "CLOUDFLARE_API_KEY"
	if input.Credential != nil {
		source = "stored credential"
	}
	return &cloudflareAuthResult{apiKey: apiKey, env: env, source: source}, nil
}

func promptCloudflareCredential(
	ctx context.Context,
	interaction AuthInteraction,
	gateway bool,
) (key, accountID, gatewayID string, err error) {
	if interaction == nil {
		return "", "", "", errors.New("auth interaction is required")
	}
	ctx = contextOrBackground(ctx)
	key, err = interaction.Prompt(ctx, AuthPrompt{
		Type:    AuthPromptSecret,
		Message: "Enter Cloudflare API key",
	})
	if err != nil {
		return "", "", "", err
	}
	accountID, err = interaction.Prompt(ctx, AuthPrompt{
		Type:    AuthPromptText,
		Message: "Enter Cloudflare account ID",
	})
	if err != nil || !gateway {
		return key, accountID, "", err
	}
	gatewayID, err = interaction.Prompt(ctx, AuthPrompt{
		Type:    AuthPromptText,
		Message: "Enter Cloudflare AI Gateway ID",
	})
	return key, accountID, gatewayID, err
}

func googleVertexAuth() *APIKeyAuth {
	return &APIKeyAuth{
		Name: "Google Cloud credentials",
		Login: func(ctx context.Context, interaction AuthInteraction) (Credential, error) {
			if interaction == nil {
				return Credential{}, errors.New("auth interaction is required")
			}
			ctx = contextOrBackground(ctx)
			method, err := interaction.Prompt(ctx, AuthPrompt{
				Type:    AuthPromptSelect,
				Message: "Select Google Vertex AI authentication method:",
				Options: []AuthPromptOption{
					{ID: "api-key", Label: "Google Cloud API key"},
					{ID: "adc", Label: "Application Default Credentials"},
					{ID: "service-account", Label: "Service account credentials file"},
				},
			})
			if err != nil {
				return Credential{}, err
			}
			if method == "api-key" {
				key, err := interaction.Prompt(ctx, AuthPrompt{
					Type:    AuthPromptSecret,
					Message: "Enter Google Cloud API key",
				})
				if err != nil {
					return Credential{}, err
				}
				return Credential{Type: CredentialTypeAPIKey, Key: key}, nil
			}
			if method != "adc" && method != "service-account" {
				return Credential{}, fmt.Errorf("unknown Google Vertex AI auth method: %s", method)
			}
			message := "Run `gcloud auth application-default login`, then provide the project and location."
			if method == "service-account" {
				message = "Provide a service account credentials file, project, and location."
			}
			interaction.Notify(AuthEvent{
				Type:    AuthEventInfo,
				Message: message,
				Links: []AuthInfoLink{{
					Label: "Application Default Credentials",
					URL:   "https://cloud.google.com/docs/authentication/provide-credentials-adc",
				}},
			})
			project, err := interaction.Prompt(ctx, AuthPrompt{
				Type:    AuthPromptText,
				Message: "Enter Google Cloud project ID",
			})
			if err != nil {
				return Credential{}, err
			}
			location, err := interaction.Prompt(ctx, AuthPrompt{
				Type:    AuthPromptText,
				Message: "Enter Google Cloud location",
			})
			if err != nil {
				return Credential{}, err
			}
			env := ProviderEnv{
				"GOOGLE_CLOUD_PROJECT":  project,
				"GOOGLE_CLOUD_LOCATION": location,
			}
			if method == "service-account" {
				path, err := interaction.Prompt(ctx, AuthPrompt{
					Type:    AuthPromptText,
					Message: "Enter service account credentials file path",
				})
				if err != nil {
					return Credential{}, err
				}
				if path != "" {
					env["GOOGLE_APPLICATION_CREDENTIALS"] = path
				}
			}
			return Credential{Type: CredentialTypeAPIKey, Env: env}, nil
		},
		Resolve: func(ctx context.Context, input APIKeyResolveInput) (*AuthResult, error) {
			ctx = contextOrBackground(ctx)
			authContext := authContextOrDefault(input.Context)
			key, err := credentialOrAmbientEnv(
				ctx,
				authContext,
				input.Credential,
				"GOOGLE_CLOUD_API_KEY",
			)
			if err != nil {
				return nil, err
			}
			if key != "" {
				source := "GOOGLE_CLOUD_API_KEY"
				if input.Credential != nil && input.Credential.Key != "" {
					source = "stored credential"
				}
				return &AuthResult{Auth: ModelAuth{APIKey: key}, Source: source}, nil
			}

			credentialsPath, err := credentialOrAmbientEnv(
				ctx,
				authContext,
				input.Credential,
				"GOOGLE_APPLICATION_CREDENTIALS",
			)
			if err != nil {
				return nil, err
			}
			if credentialsPath == "" {
				credentialsPath = vertexADCPath
			}
			hasCredentials, err := authContext.FileExists(ctx, credentialsPath)
			if err != nil {
				return nil, err
			}
			project, err := credentialOrAmbientEnv(
				ctx,
				authContext,
				input.Credential,
				"GOOGLE_CLOUD_PROJECT",
			)
			if err != nil {
				return nil, err
			}
			if project == "" {
				project, err = authEnv(ctx, authContext, "GCLOUD_PROJECT")
				if err != nil {
					return nil, err
				}
			}
			location, err := credentialOrAmbientEnv(
				ctx,
				authContext,
				input.Credential,
				"GOOGLE_CLOUD_LOCATION",
			)
			if err != nil {
				return nil, err
			}
			if !hasCredentials || project == "" || location == "" {
				return nil, nil
			}
			source := "gcloud application default credentials"
			if input.Credential != nil {
				source = "stored credential"
			}
			return &AuthResult{
				Env:    cloneCredentialEnv(input.Credential),
				Source: source,
			}, nil
		},
	}
}

func credentialOrAmbientEnv(
	ctx context.Context,
	authContext AuthContext,
	credential *Credential,
	name string,
) (string, error) {
	if credential != nil {
		if name == "CLOUDFLARE_API_KEY" || name == "GOOGLE_CLOUD_API_KEY" {
			if credential.Key != "" {
				return credential.Key, nil
			}
		}
		if value := credential.Env[name]; value != "" {
			return value, nil
		}
	}
	return authEnv(ctx, authContext, name)
}

func authEnv(ctx context.Context, authContext AuthContext, name string) (string, error) {
	value, ok, err := authContext.Env(contextOrBackground(ctx), name)
	if err != nil || !ok {
		return "", err
	}
	return value, nil
}

func authContextOrDefault(authContext AuthContext) AuthContext {
	if authContext == nil {
		return DefaultProviderAuthContext()
	}
	return authContext
}

func cloneCredentialEnv(credential *Credential) ProviderEnv {
	if credential == nil {
		return nil
	}
	return cloneProviderEnv(credential.Env)
}
