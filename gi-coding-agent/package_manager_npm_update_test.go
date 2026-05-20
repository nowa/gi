package gicodingagent

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPackageManagerNPMUpdateLookupPiParity(t *testing.T) {
	t.Run("uses npm view to fetch latest version", func(t *testing.T) {
		var gotCommand string
		var gotArgs []string
		var gotOptions PackageCommandOptions
		manager := NewDefaultPackageManager(PackageManagerOptions{
			CWD:             t.TempDir(),
			AgentDir:        t.TempDir(),
			SettingsManager: NewInMemorySettingsManager(nil),
			Operations: PackageManagerOperations{
				RunCommandCapture: func(command string, args []string, options PackageCommandOptions) (string, error) {
					gotCommand = command
					gotArgs = append([]string(nil), args...)
					gotOptions = options
					return `"1.2.3"`, nil
				},
			},
		})

		latest, err := manager.GetLatestNPMVersion("example")
		if err != nil {
			t.Fatal(err)
		}
		if latest != "1.2.3" || gotCommand != "npm" ||
			!reflect.DeepEqual(gotArgs, []string{"view", "example", "version", "--json"}) ||
			gotOptions.CWD != manager.cwd {
			t.Fatalf("latest=%q command=%q args=%#v options=%#v", latest, gotCommand, gotArgs, gotOptions)
		}
	})

	t.Run("uses npmCommand argv for npm update checks", func(t *testing.T) {
		var gotCommand string
		var gotArgs []string
		manager := NewDefaultPackageManager(PackageManagerOptions{
			CWD:      t.TempDir(),
			AgentDir: t.TempDir(),
			SettingsManager: NewInMemorySettingsManager(map[string]any{
				"npmCommand": []any{"mise", "exec", "node@20", "--", "npm"},
			}),
			Operations: PackageManagerOperations{
				RunCommandCapture: func(command string, args []string, _ PackageCommandOptions) (string, error) {
					gotCommand = command
					gotArgs = append([]string(nil), args...)
					return `"1.2.3"`, nil
				},
			},
		})

		latest, err := manager.GetLatestNPMVersion("@scope/pkg")
		if err != nil {
			t.Fatal(err)
		}
		wantArgs := []string{"exec", "node@20", "--", "npm", "view", "@scope/pkg", "version", "--json"}
		if latest != "1.2.3" || gotCommand != "mise" || !reflect.DeepEqual(gotArgs, wantArgs) {
			t.Fatalf("latest=%q command=%q args=%#v", latest, gotCommand, gotArgs)
		}
	})
}

func TestPackageManagerAvailableNPMUpdatesPiParity(t *testing.T) {
	t.Run("does not check package updates when offline", func(t *testing.T) {
		t.Setenv("GI_OFFLINE", "1")
		manager := NewDefaultPackageManager(PackageManagerOptions{
			CWD:             t.TempDir(),
			AgentDir:        t.TempDir(),
			SettingsManager: NewInMemorySettingsManager(map[string]any{"packages": []any{"npm:example"}}),
			Operations: PackageManagerOperations{
				RunCommandCapture: func(string, []string, PackageCommandOptions) (string, error) {
					t.Fatal("RunCommandCapture should not be called offline")
					return "", nil
				},
			},
		})

		updates, err := manager.CheckForAvailablePackageUpdates()
		if err != nil {
			t.Fatal(err)
		}
		if len(updates) != 0 {
			t.Fatalf("updates = %#v", updates)
		}
	})

	t.Run("reports updates for installed unpinned npm packages", func(t *testing.T) {
		agentDir, projectDir := createPackageManagerSettingsDirs(t)
		writeNPMFixturePackage(t, filepath.Join(projectDir, ConfigDirName, "npm", "node_modules", "example"), "1.0.0")
		settings := NewSettingsManager(projectDir, agentDir)
		settings.SetProjectPackages([]any{"npm:example"})
		manager := NewDefaultPackageManager(PackageManagerOptions{
			CWD:             projectDir,
			AgentDir:        agentDir,
			SettingsManager: settings,
			Operations: PackageManagerOperations{
				RunCommandCapture: func(string, []string, PackageCommandOptions) (string, error) {
					return `"1.2.3"`, nil
				},
			},
		})

		updates, err := manager.CheckForAvailablePackageUpdates()
		if err != nil {
			t.Fatal(err)
		}
		want := []PackageUpdateInfo{{Source: "npm:example", DisplayName: "example", Type: "npm", Scope: "project"}}
		if !reflect.DeepEqual(updates, want) {
			t.Fatalf("updates = %#v, want %#v", updates, want)
		}
	})

	t.Run("skips pinned packages when checking for updates", func(t *testing.T) {
		agentDir, projectDir := createPackageManagerSettingsDirs(t)
		writeNPMFixturePackage(t, filepath.Join(projectDir, ConfigDirName, "npm", "node_modules", "example"), "1.0.0")
		settings := NewSettingsManager(projectDir, agentDir)
		settings.SetProjectPackages([]any{"npm:example@1.0.0"})
		manager := NewDefaultPackageManager(PackageManagerOptions{
			CWD:             projectDir,
			AgentDir:        agentDir,
			SettingsManager: settings,
			Operations: PackageManagerOperations{
				RunCommandCapture: func(string, []string, PackageCommandOptions) (string, error) {
					t.Fatal("RunCommandCapture should not be called for pinned packages")
					return "", nil
				},
			},
		})

		updates, err := manager.CheckForAvailablePackageUpdates()
		if err != nil {
			t.Fatal(err)
		}
		if len(updates) != 0 {
			t.Fatalf("updates = %#v", updates)
		}
	})
}

func writeNPMFixturePackage(t *testing.T, dir, version string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte(`{"name":"example","version":"` + version + `"}`)
	if err := os.WriteFile(filepath.Join(dir, "package.json"), content, 0o644); err != nil {
		t.Fatal(err)
	}
}
