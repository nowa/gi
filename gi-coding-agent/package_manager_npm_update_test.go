package gicodingagent

import (
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
