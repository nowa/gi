package gicodingagent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestRefreshModelCatalogsUsesExplicitForcedNetworkPolicy(t *testing.T) {
	agentDir := t.TempDir()
	called := false
	err := refreshModelCatalogs(
		context.Background(),
		agentDir,
		func(
			ctx context.Context,
			gotAgentDir string,
			options ModelRegistryRefreshOptions,
		) (llm.ModelsRefreshResult, error) {
			called = true
			if gotAgentDir != agentDir {
				t.Fatalf("agent dir = %q, want %q", gotAgentDir, agentDir)
			}
			if !options.AllowNetwork ||
				!options.Force ||
				options.Timeout != modelCatalogRefreshTimeout {
				t.Fatalf("refresh options = %#v", options)
			}
			deadline, ok := ctx.Deadline()
			if !ok {
				t.Fatal("refresh context has no deadline")
			}
			if remaining := time.Until(deadline); remaining <= 0 ||
				remaining > modelCatalogRefreshTimeout+time.Second {
				t.Fatalf("refresh deadline remaining = %s", remaining)
			}
			return llm.ModelsRefreshResult{
				Errors: map[string]error{},
			}, nil
		},
	)
	if err != nil || !called {
		t.Fatalf("called = %t, err = %v", called, err)
	}
}

func TestRefreshModelCatalogsNormalizesFailures(t *testing.T) {
	t.Run("provider errors are deterministic", func(t *testing.T) {
		err := refreshModelCatalogs(
			context.Background(),
			t.TempDir(),
			func(
				context.Context,
				string,
				ModelRegistryRefreshOptions,
			) (llm.ModelsRefreshResult, error) {
				return llm.ModelsRefreshResult{Errors: map[string]error{
					"zeta":  errors.New("last"),
					"alpha": errors.New("first"),
				}}, nil
			},
		)
		if err == nil ||
			err.Error() != "Could not refresh model catalogs: alpha: first; zeta: last" {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("aborted refresh is a timeout", func(t *testing.T) {
		err := refreshModelCatalogs(
			context.Background(),
			t.TempDir(),
			func(
				context.Context,
				string,
				ModelRegistryRefreshOptions,
			) (llm.ModelsRefreshResult, error) {
				return llm.ModelsRefreshResult{Aborted: true}, nil
			},
		)
		if err == nil || err.Error() != "Model catalog refresh timed out." {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestCLIUpdateModelsBypassesPackageState(t *testing.T) {
	agentDir, projectDir := createPackageCommandPathDirs(t)
	var gotOptions ModelRegistryRefreshOptions
	stdout, stderr, code := runPackageCommandCLIWithOptions(
		t,
		[]string{"update", "--models"},
		projectDir,
		agentDir,
		func(options *CLIOptions) {
			options.ModelCatalogRefresh = func(
				_ context.Context,
				gotAgentDir string,
				refreshOptions ModelRegistryRefreshOptions,
			) (llm.ModelsRefreshResult, error) {
				if gotAgentDir != agentDir {
					t.Fatalf(
						"agent dir = %q, want %q",
						gotAgentDir,
						agentDir,
					)
				}
				gotOptions = refreshOptions
				return llm.ModelsRefreshResult{
					Errors: map[string]error{},
				}, nil
			}
		},
	)
	if code != 0 || stderr != "" {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if strings.TrimSpace(stdout) != "Model catalogs refreshed" {
		t.Fatalf("stdout = %q", stdout)
	}
	if !gotOptions.AllowNetwork || !gotOptions.Force {
		t.Fatalf("refresh options = %#v", gotOptions)
	}
}

func TestParseCLIUpdateArgsUsesOneTypedTarget(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want cliUpdateTarget
		note bool
	}{
		{
			name: "default self",
			want: cliUpdateTarget{kind: cliUpdateTargetSelf},
			note: true,
		},
		{
			name: "self",
			args: []string{"--self"},
			want: cliUpdateTarget{kind: cliUpdateTargetSelf},
		},
		{
			name: "extensions",
			args: []string{"--extensions"},
			want: cliUpdateTarget{kind: cliUpdateTargetExtensions},
		},
		{
			name: "combined flags",
			args: []string{"--self", "--extensions"},
			want: cliUpdateTarget{kind: cliUpdateTargetAll},
		},
		{
			name: "all",
			args: []string{"--all"},
			want: cliUpdateTarget{kind: cliUpdateTargetAll},
		},
		{
			name: "models",
			args: []string{"--models"},
			want: cliUpdateTarget{kind: cliUpdateTargetModels},
		},
		{
			name: "one extension flag",
			args: []string{"--extension", "official:gi-tools-ui"},
			want: cliUpdateTarget{
				kind:   cliUpdateTargetExtensions,
				source: "official:gi-tools-ui",
			},
		},
		{
			name: "one extension positional",
			args: []string{"git:github.com/gi/example"},
			want: cliUpdateTarget{
				kind:   cliUpdateTargetExtensions,
				source: "git:github.com/gi/example",
			},
		},
		{
			name: "self alias plus extensions",
			args: []string{"gi", "--extensions"},
			want: cliUpdateTarget{kind: cliUpdateTargetAll},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := parseCLIUpdateArgs(test.args)
			if got.target != test.want ||
				got.showExtensionsNote != test.note ||
				got.conflict != "" {
				t.Fatalf("result = %#v, want target %#v note %t", got, test.want, test.note)
			}
		})
	}
}

func TestParseCLIUpdateArgsRejectsMixedTargets(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{
			args: []string{"--models", "--self"},
			want: "--models cannot be combined with --self",
		},
		{
			args: []string{"--all", "official:gi-tools-ui"},
			want: "--all cannot be combined with a positional source",
		},
		{
			args: []string{
				"--extension",
				"official:gi-tools-ui",
				"--all",
			},
			want: "--all cannot be combined with --self",
		},
	}
	for _, test := range tests {
		got := parseCLIUpdateArgs(test.args)
		if !strings.Contains(got.conflict, test.want) {
			t.Fatalf(
				"args = %#v, conflict = %q, want %q",
				test.args,
				got.conflict,
				test.want,
			)
		}
	}
}
