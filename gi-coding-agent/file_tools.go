package gicodingagent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type FileToolOperations struct {
	Access    func(path string) error
	ReadFile  func(path string) ([]byte, error)
	WriteFile func(path string, content []byte) error
	MkdirAll  func(path string) error
}

type Edit struct {
	OldText string
	NewText string
}

type EditToolInput struct {
	Path  string
	Edits []Edit
}

type WriteToolInput struct {
	Path    string
	Content string
}

type FileToolResult struct {
	Text string
}

type EditTool struct {
	cwd string
	ops FileToolOperations
}

type WriteTool struct {
	cwd string
	ops FileToolOperations
}

func NewEditTool(cwd string, operations ...FileToolOperations) EditTool {
	return EditTool{cwd: cwd, ops: normalizeFileToolOperations(operations...)}
}

func NewWriteTool(cwd string, operations ...FileToolOperations) WriteTool {
	return WriteTool{cwd: cwd, ops: normalizeFileToolOperations(operations...)}
}

func (t EditTool) Execute(_ string, input EditToolInput) (FileToolResult, error) {
	if strings.TrimSpace(input.Path) == "" {
		return FileToolResult{}, fmt.Errorf("edit path is required")
	}
	if len(input.Edits) == 0 {
		return FileToolResult{}, fmt.Errorf("edits must contain at least one replacement")
	}
	absolutePath := ResolveToCwd(input.Path, t.cwd)
	var result FileToolResult
	err := WithFileMutationQueue(absolutePath, func() error {
		if err := t.ops.Access(absolutePath); err != nil {
			return fmt.Errorf("could not edit file: %s: %w", input.Path, err)
		}
		content, err := t.ops.ReadFile(absolutePath)
		if err != nil {
			return err
		}
		nextContent, err := applySimpleEdits(string(content), input.Edits)
		if err != nil {
			return err
		}
		if err := t.ops.WriteFile(absolutePath, []byte(nextContent)); err != nil {
			return err
		}
		result = FileToolResult{Text: fmt.Sprintf("Successfully edited %s", input.Path)}
		return nil
	})
	return result, err
}

func (t WriteTool) Execute(_ string, input WriteToolInput) (FileToolResult, error) {
	if strings.TrimSpace(input.Path) == "" {
		return FileToolResult{}, fmt.Errorf("write path is required")
	}
	absolutePath := ResolveToCwd(input.Path, t.cwd)
	err := WithFileMutationQueue(absolutePath, func() error {
		if err := t.ops.MkdirAll(filepath.Dir(absolutePath)); err != nil {
			return err
		}
		return t.ops.WriteFile(absolutePath, []byte(input.Content))
	})
	if err != nil {
		return FileToolResult{}, err
	}
	return FileToolResult{Text: fmt.Sprintf("Successfully wrote %d bytes to %s", len(input.Content), input.Path)}, nil
}

func normalizeFileToolOperations(operations ...FileToolOperations) FileToolOperations {
	ops := FileToolOperations{}
	if len(operations) > 0 {
		ops = operations[0]
	}
	if ops.Access == nil {
		ops.Access = func(path string) error {
			_, err := os.Stat(path)
			return err
		}
	}
	if ops.ReadFile == nil {
		ops.ReadFile = os.ReadFile
	}
	if ops.WriteFile == nil {
		ops.WriteFile = func(path string, content []byte) error {
			return os.WriteFile(path, content, 0o644)
		}
	}
	if ops.MkdirAll == nil {
		ops.MkdirAll = func(path string) error {
			return os.MkdirAll(path, 0o755)
		}
	}
	return ops
}

func applySimpleEdits(content string, edits []Edit) (string, error) {
	next := content
	for _, edit := range edits {
		if edit.OldText == "" {
			return "", fmt.Errorf("oldText must not be empty")
		}
		if count := strings.Count(next, edit.OldText); count != 1 {
			return "", fmt.Errorf("oldText must match exactly once, got %d matches", count)
		}
		next = strings.Replace(next, edit.OldText, edit.NewText, 1)
	}
	return next, nil
}
