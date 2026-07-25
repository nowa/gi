package gicodingagent

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestProtocolExtensionRuntimeProjectTrustFirstDecisionWins(t *testing.T) {
	runtime := NewDefaultProtocolExtensionRuntime()
	var calls []string
	mustLoadProtocolFactories(
		t,
		runtime,
		ProtocolExtensionFactory{
			Path: "failing.go",
			Factory: func(ctx *ProtocolExtensionContext) error {
				return ctx.On(
					ProtocolEventProjectTrust,
					func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
						calls = append(calls, "failing")
						if event.CWD != "/project" ||
							event.ProjectTrustContext == nil ||
							event.ProjectTrustContext.Mode != "interactive" ||
							!event.ProjectTrustContext.HasUI {
							t.Fatalf("event = %#v", event)
						}
						return ProtocolEventResult{}, errors.New("trust failure")
					},
				)
			},
		},
		ProtocolExtensionFactory{
			Path: "deciding.go",
			Factory: func(ctx *ProtocolExtensionContext) error {
				if err := ctx.On(
					ProtocolEventProjectTrust,
					func(ProtocolSessionEvent) (ProtocolEventResult, error) {
						calls = append(calls, "undecided")
						return ProtocolEventResult{
							ProjectTrust: &ProtocolProjectTrustResult{
								Trusted: ProtocolProjectTrustUndecided,
							},
						}, nil
					},
				); err != nil {
					return err
				}
				return ctx.On(
					ProtocolEventProjectTrust,
					func(ProtocolSessionEvent) (ProtocolEventResult, error) {
						calls = append(calls, "yes")
						return ProtocolEventResult{
							ProjectTrust: &ProtocolProjectTrustResult{
								Trusted:  ProtocolProjectTrustYes,
								Remember: true,
							},
						}, nil
					},
				)
			},
		},
		ProtocolExtensionFactory{
			Path: "late.go",
			Factory: func(ctx *ProtocolExtensionContext) error {
				return ctx.On(
					ProtocolEventProjectTrust,
					func(ProtocolSessionEvent) (ProtocolEventResult, error) {
						calls = append(calls, "late")
						return ProtocolEventResult{
							ProjectTrust: &ProtocolProjectTrustResult{
								Trusted: ProtocolProjectTrustNo,
							},
						}, nil
					},
				)
			},
		},
	)

	var listenerErrors []ProtocolExtensionError
	runtime.OnError(func(extensionError ProtocolExtensionError) {
		listenerErrors = append(listenerErrors, extensionError)
	})
	result, extensionErrors := runtime.EmitProjectTrustEvent(
		ProtocolProjectTrustContext{
			CWD:   "/project",
			Mode:  "interactive",
			HasUI: true,
		},
	)
	if result == nil ||
		result.Trusted != ProtocolProjectTrustYes ||
		!result.Remember {
		t.Fatalf("result = %#v", result)
	}
	if strings.Join(calls, ",") != "failing,undecided,yes" {
		t.Fatalf("calls = %#v", calls)
	}
	if len(extensionErrors) != 1 ||
		extensionErrors[0].ExtensionPath != "failing.go" ||
		!strings.Contains(extensionErrors[0].Error, "trust failure") {
		t.Fatalf("extension errors = %#v", extensionErrors)
	}
	if len(listenerErrors) != 1 ||
		listenerErrors[0] != extensionErrors[0] {
		t.Fatalf("listener errors = %#v", listenerErrors)
	}
}

func TestResolveProjectTrustedUsesExtensionBeforeSavedDecision(t *testing.T) {
	cwd := filepath.Join(t.TempDir(), "project")
	agentDir := filepath.Join(t.TempDir(), "agent")
	writeResourceFile(
		t,
		filepath.Join(cwd, ConfigDirName, "settings.json"),
		"{}",
	)
	store := NewProjectTrustStore(agentDir)
	if err := store.Set(cwd, false); err != nil {
		t.Fatal(err)
	}
	runtime := NewDefaultProtocolExtensionRuntime()
	handlerCalls := 0
	mustLoadProtocolFactories(t, runtime, ProtocolExtensionFactory{
		Path: "trust.go",
		Factory: func(ctx *ProtocolExtensionContext) error {
			return ctx.On(
				ProtocolEventProjectTrust,
				func(ProtocolSessionEvent) (ProtocolEventResult, error) {
					handlerCalls++
					return ProtocolEventResult{
						ProjectTrust: &ProtocolProjectTrustResult{
							Trusted:  ProtocolProjectTrustYes,
							Remember: true,
						},
					}, nil
				},
			)
		},
	})
	prompted := false
	trusted, err := ResolveProjectTrusted(ResolveProjectTrustOptions{
		CWD:              cwd,
		TrustStore:       store,
		ExtensionRuntime: runtime,
		ExtensionContext: ProtocolProjectTrustContext{
			CWD:  cwd,
			Mode: "print",
		},
		Prompt: func(string, []ProjectTrustOption) (*ProjectTrustOption, error) {
			prompted = true
			return nil, nil
		},
	})
	if err != nil || !trusted || handlerCalls != 1 || prompted {
		t.Fatalf(
			"trusted = %t, handler calls = %d, prompted = %t, err = %v",
			trusted,
			handlerCalls,
			prompted,
			err,
		)
	}
	if saved, found, err := store.Get(cwd); err != nil ||
		!found ||
		!saved {
		t.Fatalf("saved = %t, found = %t, err = %v", saved, found, err)
	}

	override := false
	trusted, err = ResolveProjectTrusted(ResolveProjectTrustOptions{
		CWD:              cwd,
		TrustStore:       store,
		TrustOverride:    &override,
		ExtensionRuntime: runtime,
	})
	if err != nil || trusted || handlerCalls != 1 {
		t.Fatalf(
			"override trusted = %t, handler calls = %d, err = %v",
			trusted,
			handlerCalls,
			err,
		)
	}
}

func TestDefaultResourceLoaderTrustTransactionReusesRuntimeAndFinalOrder(
	t *testing.T,
) {
	agentDir, cwd := createResourceLoaderDirs(t)
	projectExtension := filepath.Join(
		cwd,
		ConfigDirName,
		"extensions",
		"project.gi.json",
	)
	userExtension := filepath.Join(
		agentDir,
		"extensions",
		"user.gi.json",
	)
	projectSkill := filepath.Join(
		cwd,
		ConfigDirName,
		"extensions",
		"skills",
		"project",
		"SKILL.md",
	)
	writeResourceSkill(
		t,
		projectSkill,
		"project-trust-skill",
		"Project trust skill",
		"Project content",
	)
	writeJSON(t, projectExtension, map[string]any{
		"gi": map[string]any{
			"extensionProtocol": "descriptor.v1",
			"id":                "project",
			"tools": []any{map[string]any{
				"name":        "shared-tool",
				"description": "project tool",
			}},
			"resources": map[string]any{
				"skills": []any{"skills/project"},
			},
		},
	})
	writeJSON(t, userExtension, map[string]any{
		"gi": map[string]any{
			"extensionProtocol": "descriptor.v1",
			"id":                "user",
			"tools": []any{map[string]any{
				"name":        "shared-tool",
				"description": "user tool",
			}},
		},
	})

	settings := NewSettingsManagerWithOptions(
		cwd,
		agentDir,
		SettingsManagerOptions{ProjectTrusted: true},
	)
	factoryLoads := 0
	loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{
		CWD:             cwd,
		AgentDir:        agentDir,
		SettingsManager: settings,
		ExtensionFactories: []ProtocolExtensionFactory{{
			Path: "inline-trust.go",
			Factory: func(ctx *ProtocolExtensionContext) error {
				factoryLoads++
				return ctx.On(
					ProtocolEventProjectTrust,
					func(ProtocolSessionEvent) (ProtocolEventResult, error) {
						return ProtocolEventResult{
							ProjectTrust: &ProtocolProjectTrustResult{
								Trusted: ProtocolProjectTrustYes,
							},
						}, nil
					},
				)
			},
		}},
	})
	var preTrustRuntime *ProtocolExtensionRuntime
	resolveCalls := 0
	err := loader.ReloadWithOptions(ResourceLoaderReloadOptions{
		ResolveProjectTrust: func(
			input ResourceLoaderProjectTrustInput,
		) (bool, error) {
			resolveCalls++
			if settings.IsProjectTrusted() {
				t.Fatal("pre-trust settings unexpectedly trusted")
			}
			if len(input.ExtensionsResult.Extensions) != 1 ||
				input.ExtensionsResult.Extensions[0].Path != userExtension {
				t.Fatalf(
					"pre-trust extensions = %#v",
					input.ExtensionsResult.Extensions,
				)
			}
			preTrustRuntime = input.ExtensionsResult.Runtime
			return ResolveProjectTrusted(ResolveProjectTrustOptions{
				CWD:              cwd,
				TrustStore:       NewProjectTrustStore(agentDir),
				ExtensionRuntime: input.ExtensionsResult.Runtime,
				ExtensionContext: ProtocolProjectTrustContext{
					CWD:  cwd,
					Mode: "print",
				},
			})
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	extensions := loader.GetExtensions()
	if resolveCalls != 1 || factoryLoads != 1 ||
		!settings.IsProjectTrusted() {
		t.Fatalf(
			"resolve calls = %d, factory loads = %d, trusted = %t",
			resolveCalls,
			factoryLoads,
			settings.IsProjectTrusted(),
		)
	}
	if extensions.Runtime == nil ||
		extensions.Runtime != preTrustRuntime {
		t.Fatalf(
			"final runtime = %p, pre-trust runtime = %p",
			extensions.Runtime,
			preTrustRuntime,
		)
	}
	if len(extensions.Extensions) != 2 ||
		extensions.Extensions[0].Path != projectExtension ||
		extensions.Extensions[1].Path != userExtension {
		t.Fatalf("final extensions = %#v", extensions.Extensions)
	}
	tool := findDynamicSDKTool(
		extensions.Runtime.RegisteredTools(),
		"shared-tool",
	)
	if tool == nil || tool.Description != "project tool" {
		t.Fatalf(
			"visible shared tool = %#v, all = %#v",
			tool,
			extensions.Runtime.RegisteredTools(),
		)
	}
	conflictFound := false
	for _, diagnostic := range extensions.Errors {
		if diagnostic.Path == userExtension &&
			strings.Contains(
				diagnostic.Error,
				`Tool "shared-tool" conflicts with `+projectExtension,
			) {
			conflictFound = true
		}
	}
	if !conflictFound {
		t.Fatalf("errors = %#v", extensions.Errors)
	}
	if !resourceHasSkill(
		loader.GetSkills().Skills,
		"project-trust-skill",
	) {
		t.Fatalf(
			"skills = %#v, diagnostics = %#v",
			loader.GetSkills().Skills,
			loader.GetSkills().Diagnostics,
		)
	}
}

func TestDefaultResourceLoaderDoesNotRetryFailedPreTrustExtension(
	t *testing.T,
) {
	agentDir, cwd := createResourceLoaderDirs(t)
	userExtension := filepath.Join(
		agentDir,
		"extensions",
		"user.gi.json",
	)
	writeJSON(t, userExtension, map[string]any{
		"gi": map[string]any{
			"extensionProtocol": "descriptor.v1",
			"initError":         "pre-trust failure",
		},
	})
	writeResourceFile(
		t,
		filepath.Join(cwd, ConfigDirName, "settings.json"),
		"{}",
	)
	settings := NewSettingsManagerWithOptions(
		cwd,
		agentDir,
		SettingsManagerOptions{ProjectTrusted: false},
	)
	loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{
		CWD:             cwd,
		AgentDir:        agentDir,
		SettingsManager: settings,
	})
	err := loader.ReloadWithOptions(ResourceLoaderReloadOptions{
		ResolveProjectTrust: func(
			ResourceLoaderProjectTrustInput,
		) (bool, error) {
			writeJSON(t, userExtension, map[string]any{
				"gi": map[string]any{
					"extensionProtocol": "descriptor.v1",
					"commands": []any{map[string]any{
						"name": "available-after-reload",
					}},
				},
			})
			return true, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	first := loader.GetExtensions()
	if len(first.Extensions) != 0 ||
		!resourceExtensionErrorsContain(
			first.Errors,
			"pre-trust failure",
		) ||
		first.Runtime.GetCommand("available-after-reload") != nil {
		t.Fatalf("first load = %#v", first)
	}

	loader.Reload()
	second := loader.GetExtensions()
	if len(second.Extensions) != 1 ||
		second.Runtime.GetCommand("available-after-reload") == nil {
		t.Fatalf("second load = %#v", second)
	}
}

func TestDefaultResourceLoaderTrustResolverFailureKeepsPublishedState(
	t *testing.T,
) {
	agentDir, cwd := createResourceLoaderDirs(t)
	projectExtension := filepath.Join(
		cwd,
		ConfigDirName,
		"extensions",
		"project.gi.json",
	)
	writeGiProtocolExtensionDescriptor(t, projectExtension)
	settings := NewSettingsManagerWithOptions(
		cwd,
		agentDir,
		SettingsManagerOptions{ProjectTrusted: true},
	)
	loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{
		CWD:             cwd,
		AgentDir:        agentDir,
		SettingsManager: settings,
	})
	loader.Reload()
	before := loader.GetExtensions()
	if len(before.Extensions) != 1 {
		t.Fatalf("before = %#v", before)
	}

	wantError := errors.New("trust resolver stopped")
	err := loader.ReloadWithOptions(ResourceLoaderReloadOptions{
		ResolveProjectTrust: func(
			ResourceLoaderProjectTrustInput,
		) (bool, error) {
			return false, wantError
		},
	})
	if !errors.Is(err, wantError) {
		t.Fatalf("err = %v", err)
	}
	after := loader.GetExtensions()
	if !settings.IsProjectTrusted() ||
		after.Runtime != before.Runtime ||
		len(after.Extensions) != 1 ||
		after.Extensions[0].Path != projectExtension {
		t.Fatalf(
			"trusted = %t, before = %#v, after = %#v",
			settings.IsProjectTrusted(),
			before,
			after,
		)
	}
}

func TestProtocolExtensionLoaderGenerationRejectsStaleCacheWrites(
	t *testing.T,
) {
	cwd := t.TempDir()
	path := filepath.Join(cwd, "cached.gi.json")
	writeJSON(t, path, map[string]any{
		"gi": map[string]any{
			"extensionProtocol": "descriptor.v1",
			"id":                "first",
		},
	})
	loader := newProtocolExtensionLoader()
	staleToken := loader.useExtensionCacheCWD(cwd)
	first, err := loader.loadDescriptor(path, &staleToken)
	if err != nil || first.Gi.ID != "first" {
		t.Fatalf("first descriptor = %#v, err = %v", first, err)
	}

	loader.clearExtensionCache()
	writeJSON(t, path, map[string]any{
		"gi": map[string]any{
			"extensionProtocol": "descriptor.v1",
			"id":                "second",
		},
	})
	staleRead, err := loader.loadDescriptor(path, &staleToken)
	if err != nil || staleRead.Gi.ID != "second" {
		t.Fatalf("stale read = %#v, err = %v", staleRead, err)
	}
	writeJSON(t, path, map[string]any{
		"gi": map[string]any{
			"extensionProtocol": "descriptor.v1",
			"id":                "third",
		},
	})
	currentToken := loader.useExtensionCacheCWD(cwd)
	current, err := loader.loadDescriptor(path, &currentToken)
	if err != nil || current.Gi.ID != "third" {
		t.Fatalf("current descriptor = %#v, err = %v", current, err)
	}

	writeJSON(t, path, map[string]any{
		"gi": map[string]any{
			"extensionProtocol": "descriptor.v1",
			"id":                "fourth",
		},
	})
	cached, err := loader.loadDescriptor(path, &currentToken)
	if err != nil || cached.Gi.ID != "third" {
		t.Fatalf("cached descriptor = %#v, err = %v", cached, err)
	}
	loader.clearExtensionCache()
	refreshedToken := loader.useExtensionCacheCWD(cwd)
	refreshed, err := loader.loadDescriptor(path, &refreshedToken)
	if err != nil || refreshed.Gi.ID != "fourth" {
		t.Fatalf("refreshed descriptor = %#v, err = %v", refreshed, err)
	}
}
