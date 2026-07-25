package gicodingagent

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestResourceConfigSnapshotSeparatesGlobalAndProjectViews(t *testing.T) {
	agentDir, projectDir := createPackageManagerSettingsDirs(t)
	userSkill := filepath.Join(agentDir, "skills", "user-review", "SKILL.md")
	projectPrompt := filepath.Join(projectDir, ConfigDirName, "prompts", "ship.md")
	writeResourceSkill(t, userSkill, "user-review", "User Review", "Review changes.")
	writeResourceFile(t, projectPrompt, "# Ship")

	settings := NewSettingsManager(projectDir, agentDir)
	manager := NewDefaultPackageManager(PackageManagerOptions{
		CWD:             projectDir,
		AgentDir:        agentDir,
		SettingsManager: settings,
	})
	snapshot, err := manager.LoadResourceConfigSnapshot()
	if err != nil {
		t.Fatal(err)
	}

	global := snapshot.Items(ResourceConfigWriteGlobal)
	project := snapshot.Items(ResourceConfigWriteProject)
	userItem, ok := resourceConfigTestItem(global, "skills", userSkill)
	if !ok {
		t.Fatalf("global items = %#v", global)
	}
	if _, ok := resourceConfigTestItem(global, "prompts", projectPrompt); ok {
		t.Fatalf("global projection leaked project prompt: %#v", global)
	}
	if _, ok := resourceConfigTestItem(project, "skills", userSkill); !ok {
		t.Fatalf("project projection missing inherited skill: %#v", project)
	}
	if _, ok := resourceConfigTestItem(project, "prompts", projectPrompt); !ok {
		t.Fatalf("project projection missing local prompt: %#v", project)
	}
	if inheritedEnabled, inherited := snapshot.InheritedEnabled(userItem); !inherited || !inheritedEnabled {
		t.Fatalf("inherited state = (%v, %v), want (true, true)", inheritedEnabled, inherited)
	}
	if !snapshot.IsInherited(userItem) {
		t.Fatal("user item should be inherited")
	}

	global[0].Enabled = false
	if fresh := snapshot.Items(ResourceConfigWriteGlobal); !fresh[0].Enabled {
		t.Fatal("Items returned mutable snapshot storage")
	}
}

func TestNextProjectResourceOverrideStatePiParity(t *testing.T) {
	tests := []struct {
		name      string
		current   ProjectResourceOverrideState
		inherited bool
		want      ProjectResourceOverrideState
	}{
		{name: "enabled inherit unloads", current: ProjectResourceInherit, inherited: true, want: ProjectResourceUnload},
		{name: "enabled unload loads", current: ProjectResourceUnload, inherited: true, want: ProjectResourceLoad},
		{name: "enabled load inherits", current: ProjectResourceLoad, inherited: true, want: ProjectResourceInherit},
		{name: "disabled inherit loads", current: ProjectResourceInherit, inherited: false, want: ProjectResourceLoad},
		{name: "disabled load unloads", current: ProjectResourceLoad, inherited: false, want: ProjectResourceUnload},
		{name: "disabled unload inherits", current: ProjectResourceUnload, inherited: false, want: ProjectResourceInherit},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := NextProjectResourceOverrideState(test.current, test.inherited); got != test.want {
				t.Fatalf("next state = %q, want %q", got, test.want)
			}
		})
	}
}

func TestProjectTopLevelResourceOverrideLifecycle(t *testing.T) {
	agentDir, projectDir := createPackageManagerSettingsDirs(t)
	userSkill := filepath.Join(agentDir, "skills", "review", "SKILL.md")
	writeResourceSkill(t, userSkill, "review", "Review", "Review changes.")
	settings := NewSettingsManager(projectDir, agentDir)
	manager := NewDefaultPackageManager(PackageManagerOptions{
		CWD:             projectDir,
		AgentDir:        agentDir,
		SettingsManager: settings,
	})
	snapshot, err := manager.LoadResourceConfigSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	item, ok := resourceConfigTestItem(snapshot.Items(ResourceConfigWriteGlobal), "skills", userSkill)
	if !ok {
		t.Fatalf("global items = %#v", snapshot.Items(ResourceConfigWriteGlobal))
	}

	if state := manager.ProjectResourceOverrideState(item); state != ProjectResourceInherit {
		t.Fatalf("initial state = %q", state)
	}
	target := snapshot.Target(item)
	if changed, err := manager.SetProjectResourceOverride(target, ProjectResourceUnload); err != nil || !changed {
		t.Fatalf("set unload = (%v, %v)", changed, err)
	}
	if got := settingsStringSlice(settings.GetProjectSettings(), "skills"); !reflect.DeepEqual(got, []string{userSkill, "-" + userSkill}) {
		t.Fatalf("project skills = %#v", got)
	}
	if state := manager.ProjectResourceOverrideState(item); state != ProjectResourceUnload {
		t.Fatalf("unload state = %q", state)
	}

	if changed, err := manager.SetProjectResourceOverride(target, ProjectResourceLoad); err != nil || !changed {
		t.Fatalf("set load = (%v, %v)", changed, err)
	}
	if got := settingsStringSlice(settings.GetProjectSettings(), "skills"); !reflect.DeepEqual(got, []string{userSkill, "+" + userSkill}) {
		t.Fatalf("project skills = %#v", got)
	}
	if state := manager.ProjectResourceOverrideState(item); state != ProjectResourceLoad {
		t.Fatalf("load state = %q", state)
	}

	if changed, err := manager.SetProjectResourceOverride(target, ProjectResourceInherit); err != nil || !changed {
		t.Fatalf("set inherit = (%v, %v)", changed, err)
	}
	if got := settingsStringSlice(settings.GetProjectSettings(), "skills"); len(got) != 0 {
		t.Fatalf("project skills = %#v, want empty", got)
	}
	if state := manager.ProjectResourceOverrideState(item); state != ProjectResourceInherit {
		t.Fatalf("inherited state = %q", state)
	}
}

func TestProjectPackageResourceOverrideLifecycle(t *testing.T) {
	agentDir, projectDir := createPackageManagerSettingsDirs(t)
	pkgDir := filepath.Join(projectDir, "shared-package")
	skillPath := filepath.Join(pkgDir, "skills", "review", "SKILL.md")
	writeResourceSkill(t, skillPath, "review", "Review", "Review changes.")
	globalSource, err := filepath.Rel(agentDir, pkgDir)
	if err != nil {
		t.Fatal(err)
	}
	settings := NewSettingsManager(projectDir, agentDir)
	settings.SetPackages([]any{globalSource})
	manager := NewDefaultPackageManager(PackageManagerOptions{
		CWD:             projectDir,
		AgentDir:        agentDir,
		SettingsManager: settings,
	})
	snapshot, err := manager.LoadResourceConfigSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	item, ok := resourceConfigTestItem(snapshot.Items(ResourceConfigWriteGlobal), "skills", skillPath)
	if !ok {
		t.Fatalf("global items = %#v", snapshot.Items(ResourceConfigWriteGlobal))
	}

	target := snapshot.Target(item)
	if changed, err := manager.SetProjectResourceOverride(target, ProjectResourceUnload); err != nil || !changed {
		t.Fatalf("set unload = (%v, %v)", changed, err)
	}
	packages := settingsSlice(settings.GetProjectSettings(), "packages")
	if len(packages) != 1 {
		t.Fatalf("project packages = %#v", packages)
	}
	object, ok := packages[0].(map[string]any)
	if !ok {
		t.Fatalf("project package = %#v", packages[0])
	}
	projectSource, err := filepath.Rel(filepath.Join(projectDir, ConfigDirName), pkgDir)
	if err != nil {
		t.Fatal(err)
	}
	if object["source"] != projectSource || object["autoload"] != false ||
		!reflect.DeepEqual(settingsStringSlice(object, "skills"), []string{"-skills/review/SKILL.md"}) {
		t.Fatalf("project package = %#v", object)
	}
	if state := manager.ProjectResourceOverrideState(item); state != ProjectResourceUnload {
		t.Fatalf("unload state = %q", state)
	}
	resolved, err := manager.ResolveConfiguredProtocolPackageResources()
	if err != nil {
		t.Fatal(err)
	}
	if !protocolPackagePathDisabled(resolved.Skills, skillPath) {
		t.Fatalf("resolved skills = %#v", resolved.Skills)
	}

	if changed, err := manager.SetProjectResourceOverride(target, ProjectResourceLoad); err != nil || !changed {
		t.Fatalf("set load = (%v, %v)", changed, err)
	}
	if state := manager.ProjectResourceOverrideState(item); state != ProjectResourceLoad {
		t.Fatalf("load state = %q", state)
	}
	packages = settingsSlice(settings.GetProjectSettings(), "packages")
	object = packages[0].(map[string]any)
	if got := settingsStringSlice(object, "skills"); !reflect.DeepEqual(got, []string{"+skills/review/SKILL.md"}) {
		t.Fatalf("skill overrides = %#v", got)
	}

	if changed, err := manager.SetProjectResourceOverride(target, ProjectResourceInherit); err != nil || !changed {
		t.Fatalf("set inherit = (%v, %v)", changed, err)
	}
	if packages := settingsSlice(settings.GetProjectSettings(), "packages"); len(packages) != 0 {
		t.Fatalf("project packages = %#v, want empty", packages)
	}
	if state := manager.ProjectResourceOverrideState(item); state != ProjectResourceInherit {
		t.Fatalf("inherit state = %q", state)
	}
}

func TestProjectPackageEmptyFilterStateDependsOnAutoload(t *testing.T) {
	agentDir, projectDir := createPackageManagerSettingsDirs(t)
	pkgDir := filepath.Join(projectDir, "package")
	item := PackageResourceToggleItem{
		Source:       pkgDir,
		Scope:        "project",
		ResourceType: "skills",
		Pattern:      "skills/review/SKILL.md",
		Path:         filepath.Join(pkgDir, "skills", "review", "SKILL.md"),
		Metadata:     ProtocolSourceInfo{Origin: "package", Scope: "project"},
	}
	settings := NewSettingsManager(projectDir, agentDir)
	manager := NewDefaultPackageManager(PackageManagerOptions{
		CWD:             projectDir,
		AgentDir:        agentDir,
		SettingsManager: settings,
	})

	if err := settings.SetProjectPackages([]any{map[string]any{
		"source": pkgDir,
		"skills": []any{},
	}}); err != nil {
		t.Fatal(err)
	}
	if state := manager.ProjectResourceOverrideState(item); state != ProjectResourceUnload {
		t.Fatalf("autoload default empty state = %q", state)
	}

	if err := settings.SetProjectPackages([]any{map[string]any{
		"source":   pkgDir,
		"autoload": false,
		"skills":   []any{},
	}}); err != nil {
		t.Fatal(err)
	}
	if state := manager.ProjectResourceOverrideState(item); state != ProjectResourceInherit {
		t.Fatalf("autoload=false empty state = %q", state)
	}
}

func resourceConfigTestItem(items []PackageResourceToggleItem, resourceType, path string) (PackageResourceToggleItem, bool) {
	for _, item := range items {
		if item.ResourceType == resourceType && CanonicalizePath(item.Path) == CanonicalizePath(path) {
			return item, true
		}
	}
	return PackageResourceToggleItem{}, false
}
