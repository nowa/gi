package gicodingagent

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestEditToolPiAccessErrors(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "edit-readonly.txt")
	writeEditToolFile(t, testFile, "hello\n")

	permissionTool := NewEditTool(dir, FileToolOperations{
		WriteFile: func(_ string, _ []byte) error {
			return os.ErrPermission
		},
	})
	_, err := permissionTool.Execute("test-call-14", EditToolInput{
		Path:  testFile,
		Edits: []Edit{{OldText: "hello", NewText: "world"}},
	})
	if err == nil || err.Error() != "Could not edit file: "+testFile+". Error code: EACCES." {
		t.Fatalf("permission err = %v", err)
	}

	genericFailureTool := NewEditTool(dir, FileToolOperations{
		Access: func(_ string) error {
			return errors.New("disk offline")
		},
		ReadFile: func(_ string) ([]byte, error) {
			return []byte("hello\n"), nil
		},
		WriteFile: func(_ string, _ []byte) error {
			return nil
		},
	})
	_, err = genericFailureTool.Execute("test-call-16", EditToolInput{
		Path:  "broken.txt",
		Edits: []Edit{{OldText: "hello", NewText: "world"}},
	})
	if err == nil || err.Error() != "Could not edit file: broken.txt. Error: disk offline." {
		t.Fatalf("generic err = %v", err)
	}
}

func TestComputeEditsDiffPiAccessErrors(t *testing.T) {
	dir := t.TempDir()
	missingFile := filepath.Join(dir, "missing-preview.txt")
	missingResult := ComputeEditsDiff(missingFile, []Edit{{OldText: "hello", NewText: "world"}}, dir)
	if missingResult != (EditDiffResult{Error: "Could not edit file: " + missingFile + ". Error code: ENOENT."}) {
		t.Fatalf("missing result = %#v", missingResult)
	}

	unreadableFile := filepath.Join(dir, "unreadable-preview.txt")
	writeEditToolFile(t, unreadableFile, "hello\n")
	unreadableResult := ComputeEditsDiff(unreadableFile, []Edit{{OldText: "hello", NewText: "world"}}, dir, FileToolOperations{
		ReadFile: func(_ string) ([]byte, error) {
			return nil, os.ErrPermission
		},
	})
	if unreadableResult != (EditDiffResult{Error: "Could not edit file: " + unreadableFile + ". Error code: EACCES."}) {
		t.Fatalf("unreadable result = %#v", unreadableResult)
	}
}
