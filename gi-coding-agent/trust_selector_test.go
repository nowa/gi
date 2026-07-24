package gicodingagent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTrustSelectorMarksTheSavedTrustedDecision(t *testing.T) {
	cwd := existingTrustSelectorCWD(t)
	options := GetProjectTrustOptions(cwd, false)
	dialogOptions, selected := projectTrustDialogOptions(options, &ProjectTrustStoreEntry{
		Path:     normalizeProjectTrustPath(cwd),
		Decision: true,
	})
	if selected != 0 || !strings.Contains(dialogOptions[selected].Label, "Trust ✓") {
		t.Fatalf("dialog options = %#v, selected = %d", dialogOptions, selected)
	}
	for index, option := range dialogOptions {
		if index != selected && strings.Contains(option.Label, "✓") {
			t.Fatalf("unexpected saved marker in %#v", dialogOptions)
		}
	}
}

func TestTrustSelectorSelectsATrustDecision(t *testing.T) {
	cwd := existingTrustSelectorCWD(t)
	options := GetProjectTrustOptions(cwd, false)
	if len(options) == 0 {
		t.Fatal("trust options are empty")
	}
	selected := options[0]
	if !selected.Trusted || len(selected.Updates) != 1 ||
		selected.Updates[0].Path != normalizeProjectTrustPath(cwd) ||
		selected.Updates[0].Decision == nil || !*selected.Updates[0].Decision {
		t.Fatalf("selected option = %#v", selected)
	}
}

func TestTrustSelectorLabelsSavedAncestorDecisionsAsInherited(t *testing.T) {
	cwd := existingTrustSelectorCWD(t)
	parent := normalizeProjectTrustPath(filepath.Dir(cwd))
	label := projectTrustSavedDecisionLabel(cwd, &ProjectTrustStoreEntry{
		Path:     parent,
		Decision: true,
	})
	if !strings.Contains(label, "trusted") || !strings.Contains(label, "inherited from "+parent) {
		t.Fatalf("label = %q", label)
	}
}

func TestTrustSelectorAddsATrustParentOption(t *testing.T) {
	cwd := existingTrustSelectorCWD(t)
	parent := normalizeProjectTrustPath(filepath.Dir(cwd))
	options := GetProjectTrustOptions(cwd, false)
	dialogOptions, selected := projectTrustDialogOptions(options, &ProjectTrustStoreEntry{
		Path:     parent,
		Decision: true,
	})
	if selected < 0 || selected >= len(options) ||
		!strings.Contains(dialogOptions[selected].Label, "Trust parent folder ("+parent+") ✓") {
		t.Fatalf("dialog options = %#v, selected = %d", dialogOptions, selected)
	}
	option := options[selected]
	if !option.Trusted || len(option.Updates) != 2 ||
		option.Updates[0].Path != parent || option.Updates[0].Decision == nil ||
		!*option.Updates[0].Decision ||
		option.Updates[1].Path != normalizeProjectTrustPath(cwd) ||
		option.Updates[1].Decision != nil {
		t.Fatalf("parent option = %#v", option)
	}
}

func existingTrustSelectorCWD(t *testing.T) string {
	t.Helper()
	cwd := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	return cwd
}
