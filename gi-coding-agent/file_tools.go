package gicodingagent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	agentharness "github.com/nowa/gi/gi-agent-core/harness"
	harnessenv "github.com/nowa/gi/gi-agent-core/harness/env"
	harnesstools "github.com/nowa/gi/gi-agent-core/harness/tools"
	llm "github.com/nowa/gi/gi-llm-provider"
)

const (
	defaultReadToolLineLimit = agentharness.DefaultMaxLines
	defaultReadToolByteLimit = agentharness.DefaultMaxBytes
)

type FileToolOperations struct {
	Access      func(path string) error
	ReadFile    func(path string) ([]byte, error)
	WriteFile   func(path string, content []byte) error
	MkdirAll    func(path string) error
	ResizeImage func(part llm.ContentPart, options ImageResizeOptions) *ResizedImage
}

type Edit = harnesstools.Edit

type EditToolInput struct {
	Path  string
	Edits []Edit
}

type WriteToolInput struct {
	Path    string
	Content string
}

type ReadToolInput struct {
	Path   string
	Offset int
	Limit  int
}

type FileToolResult struct {
	Text    string
	Content []llm.ContentPart
	Details *FileToolDetails
}

type FileToolDetails struct {
	Truncation           *ReadToolTruncation `json:"truncation,omitempty"`
	Diff                 string              `json:"diff,omitempty"`
	Patch                string              `json:"patch,omitempty"`
	FirstChangedLine     int                 `json:"firstChangedLine,omitempty"`
	FullOutputPath       string              `json:"fullOutputPath,omitempty"`
	MatchLimitReached    int                 `json:"matchLimitReached,omitempty"`
	ResultLimitReached   int                 `json:"resultLimitReached,omitempty"`
	EntryLimitReached    int                 `json:"entryLimitReached,omitempty"`
	SearchLinesTruncated bool                `json:"searchLinesTruncated,omitempty"`
}

type EditDiffResult struct {
	Diff  string `json:"diff,omitempty"`
	Error string `json:"error,omitempty"`
}

type ReadToolTruncation = agentharness.TruncationResult

type EditTool struct {
	cwd string
	ops FileToolOperations
}

type WriteTool struct {
	cwd string
	ops FileToolOperations
}

type ReadTool struct {
	cwd              string
	ops              FileToolOperations
	autoResizeImages bool
}

type ReadToolOptions struct {
	AutoResizeImages *bool
}

var codingFileMutations = harnesstools.NewFileMutationQueue()

func NewEditTool(cwd string, operations ...FileToolOperations) EditTool {
	return EditTool{cwd: cwd, ops: normalizeFileToolOperations(operations...)}
}

func NewWriteTool(cwd string, operations ...FileToolOperations) WriteTool {
	return WriteTool{cwd: cwd, ops: normalizeFileToolOperations(operations...)}
}

func NewReadTool(cwd string, operations ...FileToolOperations) ReadTool {
	return NewReadToolWithOptions(cwd, ReadToolOptions{}, operations...)
}

func NewReadToolWithOptions(cwd string, options ReadToolOptions, operations ...FileToolOperations) ReadTool {
	autoResizeImages := true
	if options.AutoResizeImages != nil {
		autoResizeImages = *options.AutoResizeImages
	}
	return ReadTool{
		cwd:              cwd,
		ops:              normalizeFileToolOperations(operations...),
		autoResizeImages: autoResizeImages,
	}
}

func (t EditTool) Execute(toolCallID string, input EditToolInput) (FileToolResult, error) {
	if strings.TrimSpace(input.Path) == "" {
		return FileToolResult{}, fmt.Errorf("edit path is required")
	}
	if len(input.Edits) == 0 {
		return FileToolResult{}, fmt.Errorf("edits must contain at least one replacement")
	}
	rawEdits := make([]any, 0, len(input.Edits))
	for _, edit := range input.Edits {
		rawEdits = append(rawEdits, map[string]any{
			"oldText": edit.OldText,
			"newText": edit.NewText,
		})
	}
	return executeCompatibilityFileTool(
		harnesstools.CreateEditTool(),
		toolCallID,
		map[string]any{"path": input.Path, "edits": rawEdits},
		t.executionContext(),
	)
}

func (t WriteTool) Execute(toolCallID string, input WriteToolInput) (FileToolResult, error) {
	if strings.TrimSpace(input.Path) == "" {
		return FileToolResult{}, fmt.Errorf("write path is required")
	}
	return executeCompatibilityFileTool(
		harnesstools.CreateWriteTool(),
		toolCallID,
		map[string]any{"path": input.Path, "content": input.Content},
		t.executionContext(),
	)
}

func (t ReadTool) Execute(toolCallID string, input ReadToolInput) (FileToolResult, error) {
	if strings.TrimSpace(input.Path) == "" {
		return FileToolResult{}, fmt.Errorf("read path is required")
	}
	autoResizeImages := t.autoResizeImages
	tool := harnesstools.CreateReadTool(harnesstools.ReadToolOptions{
		AutoResizeImages: &autoResizeImages,
		ImageProcessor:   codingReadImageProcessor(t.ops),
	})
	params := map[string]any{"path": input.Path}
	if input.Offset > 0 {
		params["offset"] = input.Offset
	}
	if input.Limit > 0 {
		params["limit"] = input.Limit
	}
	return executeCompatibilityFileTool(tool, toolCallID, params, t.executionContext())
}

func (t EditTool) executionContext() harnesstools.ExecutionToolContext {
	return compatibilityExecutionContext(t.cwd, t.ops)
}

func (t WriteTool) executionContext() harnesstools.ExecutionToolContext {
	return compatibilityExecutionContext(t.cwd, t.ops)
}

func (t ReadTool) executionContext() harnesstools.ExecutionToolContext {
	return compatibilityExecutionContext(t.cwd, t.ops)
}

func compatibilityExecutionContext(cwd string, ops FileToolOperations) harnesstools.ExecutionToolContext {
	return harnesstools.ExecutionToolContext{
		Env:       &fileToolExecutionEnv{cwd: cwd, ops: ops},
		Mutations: codingFileMutations,
	}
}

func executeCompatibilityFileTool(
	tool agentharness.AgentHarnessTool,
	toolCallID string,
	params map[string]any,
	toolContext harnesstools.ExecutionToolContext,
) (FileToolResult, error) {
	result, err := tool.Execute(context.Background(), toolCallID, params, nil, toolContext)
	if err != nil {
		return FileToolResult{}, err
	}
	compatibilityResult := FileToolResult{
		Text:    textFromContentParts(result.Content),
		Content: append([]llm.ContentPart(nil), result.Content...),
	}
	switch details := result.Details.(type) {
	case *harnesstools.ReadToolDetails:
		if details != nil {
			compatibilityResult.Details = &FileToolDetails{Truncation: details.Truncation}
		}
	case *harnesstools.EditToolDetails:
		if details != nil {
			compatibilityResult.Details = &FileToolDetails{
				Diff:             details.Diff,
				Patch:            details.Patch,
				FirstChangedLine: details.FirstChangedLine,
			}
		}
	}
	return compatibilityResult, nil
}

func textFromContentParts(content []llm.ContentPart) string {
	var values []string
	for _, part := range content {
		if part.Type == llm.ContentText {
			values = append(values, part.Text)
		}
	}
	return strings.Join(values, "\n")
}

func codingReadImageProcessor(ops FileToolOperations) harnesstools.ReadImageProcessor {
	return func(_ context.Context, content []byte, mimeType string, autoResizeImages bool) (harnesstools.ReadImageProcessorResult, error) {
		processed := processImageWithResize(content, mimeType, autoResizeImages, ops.ResizeImage)
		if !processed.OK {
			return harnesstools.ReadImageProcessorResult{
				Message: processed.Message,
			}, nil
		}
		return harnesstools.ReadImageProcessorResult{
			OK:       true,
			Data:     processed.Data,
			MIMEType: processed.MIMEType,
			Hints:    append([]string(nil), processed.Hints...),
		}, nil
	}
}

func ComputeEditsDiff(path string, edits []Edit, cwd string, operations ...FileToolOperations) EditDiffResult {
	if strings.TrimSpace(path) == "" {
		return EditDiffResult{Error: "edit path is required"}
	}
	if len(edits) == 0 {
		return EditDiffResult{Error: "edits must contain at least one replacement"}
	}
	ops := normalizeFileToolOperations(operations...)
	absolutePath := ResolveToCwd(path, cwd)
	if err := ops.Access(absolutePath); err != nil {
		return EditDiffResult{Error: formatEditAccessError(path, err).Error()}
	}
	content, err := ops.ReadFile(absolutePath)
	if err != nil {
		return EditDiffResult{Error: formatEditAccessError(path, err).Error()}
	}
	_, text := harnesstools.StripBOM(string(content))
	applied, err := harnesstools.ApplyEditsToNormalizedContent(harnesstools.NormalizeToLF(text), edits, path)
	if err != nil {
		return EditDiffResult{Error: err.Error()}
	}
	return EditDiffResult{Diff: applied.Diff}
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
	if ops.ResizeImage == nil {
		ops.ResizeImage = func(part llm.ContentPart, options ImageResizeOptions) *ResizedImage {
			return ResizeImage(part, options)
		}
	}
	return ops
}

func formatEditAccessError(path string, err error) error {
	switch {
	case os.IsNotExist(err):
		return fmt.Errorf("Could not edit file: %s. Error code: ENOENT.", path)
	case os.IsPermission(err):
		return fmt.Errorf("Could not edit file: %s. Error code: EACCES.", path)
	default:
		return fmt.Errorf("Could not edit file: %s. Error: %s.", path, err.Error())
	}
}

func detectSupportedImageMIMEType(content []byte) string {
	return harnesstools.DetectSupportedImageMIMEType(content)
}

type fileToolExecutionEnv struct {
	cwd string
	ops FileToolOperations
}

func (e *fileToolExecutionEnv) CWD() string {
	return e.cwd
}

func (e *fileToolExecutionEnv) AbsolutePath(path string) string {
	return ResolveToCwd(path, e.cwd)
}

func (e *fileToolExecutionEnv) JoinPath(parts ...string) string {
	return filepath.Join(parts...)
}

func (e *fileToolExecutionEnv) ReadTextFile(ctx context.Context, path string) (string, error) {
	content, err := e.ReadBinaryFile(ctx, path)
	return string(content), err
}

func (e *fileToolExecutionEnv) ReadTextLines(ctx context.Context, path string, maxLines int) ([]string, error) {
	content, err := e.ReadTextFile(ctx, path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(content, "\n")
	if maxLines > 0 && len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	return lines, nil
}

func (e *fileToolExecutionEnv) ReadBinaryFile(ctx context.Context, path string) ([]byte, error) {
	if err := compatibilityContextError(ctx, path); err != nil {
		return nil, err
	}
	content, err := e.ops.ReadFile(e.AbsolutePath(path))
	if err != nil {
		return nil, compatibilityFileError(path, err)
	}
	return content, nil
}

func (e *fileToolExecutionEnv) WriteFile(ctx context.Context, path string, content []byte) error {
	if err := compatibilityContextError(ctx, path); err != nil {
		return err
	}
	absolutePath := e.AbsolutePath(path)
	if err := e.ops.MkdirAll(filepath.Dir(absolutePath)); err != nil {
		return compatibilityFileError(path, err)
	}
	if err := e.ops.WriteFile(absolutePath, content); err != nil {
		return compatibilityFileError(path, err)
	}
	return nil
}

func (e *fileToolExecutionEnv) AppendFile(ctx context.Context, path string, content []byte) error {
	existing, err := e.ReadBinaryFile(ctx, path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return e.WriteFile(ctx, path, append(existing, content...))
}

func (e *fileToolExecutionEnv) FileInfo(ctx context.Context, path string) (harnessenv.FileInfo, error) {
	if err := compatibilityContextError(ctx, path); err != nil {
		return harnessenv.FileInfo{}, err
	}
	absolutePath := e.AbsolutePath(path)
	if err := e.ops.Access(absolutePath); err != nil {
		return harnessenv.FileInfo{}, compatibilityFileError(path, err)
	}
	if info, err := os.Lstat(absolutePath); err == nil {
		kind := harnessenv.FileKindFile
		if info.IsDir() {
			kind = harnessenv.FileKindDirectory
		} else if info.Mode()&os.ModeSymlink != 0 {
			kind = harnessenv.FileKindSymlink
		}
		return harnessenv.FileInfo{
			Name:    info.Name(),
			Path:    absolutePath,
			Kind:    kind,
			Size:    info.Size(),
			ModTime: info.ModTime(),
		}, nil
	}
	return harnessenv.FileInfo{
		Name: filepath.Base(absolutePath),
		Path: absolutePath,
		Kind: harnessenv.FileKindFile,
	}, nil
}

func (e *fileToolExecutionEnv) ListDir(context.Context, string) ([]harnessenv.FileInfo, error) {
	return nil, compatibilityNotSupported("list directory")
}

func (e *fileToolExecutionEnv) CanonicalPath(ctx context.Context, path string) (string, error) {
	if err := compatibilityContextError(ctx, path); err != nil {
		return "", err
	}
	absolutePath := e.AbsolutePath(path)
	canonicalPath, err := filepath.EvalSymlinks(absolutePath)
	if err != nil {
		code := harnessenv.FileErrorUnknown
		if os.IsNotExist(err) {
			code = harnessenv.FileErrorNotFound
		}
		return "", &harnessenv.FileError{Code: code, Path: absolutePath, Err: err}
	}
	return canonicalPath, nil
}

func (e *fileToolExecutionEnv) Exists(ctx context.Context, path string) (bool, error) {
	if err := compatibilityContextError(ctx, path); err != nil {
		return false, err
	}
	err := e.ops.Access(e.AbsolutePath(path))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, compatibilityFileError(path, err)
}

func (e *fileToolExecutionEnv) CreateDir(ctx context.Context, path string, _ harnessenv.CreateDirOptions) error {
	if err := compatibilityContextError(ctx, path); err != nil {
		return err
	}
	return e.ops.MkdirAll(e.AbsolutePath(path))
}

func (e *fileToolExecutionEnv) Remove(context.Context, string, harnessenv.RemoveOptions) error {
	return compatibilityNotSupported("remove file")
}

func (e *fileToolExecutionEnv) CreateTempDir(context.Context, string) (string, error) {
	return "", compatibilityNotSupported("create temporary directory")
}

func (e *fileToolExecutionEnv) CreateTempFile(context.Context, harnessenv.TempFileOptions) (string, error) {
	return "", compatibilityNotSupported("create temporary file")
}

func (e *fileToolExecutionEnv) Cleanup(context.Context) error {
	return nil
}

func (e *fileToolExecutionEnv) Exec(context.Context, string, harnessenv.ExecOptions) (harnessenv.ExecResult, error) {
	return harnessenv.ExecResult{}, &harnessenv.ExecutionError{
		Code: harnessenv.ExecutionErrorShellUnavailable,
		Err:  errors.New("shell execution is not available for compatibility file tools"),
	}
}

func compatibilityContextError(ctx context.Context, path string) error {
	if ctx == nil || ctx.Err() == nil {
		return nil
	}
	return &harnessenv.FileError{
		Code: harnessenv.FileErrorAborted,
		Path: path,
		Err:  ctx.Err(),
	}
}

func compatibilityFileError(path string, err error) error {
	if err == nil {
		return nil
	}
	code := ""
	switch {
	case os.IsNotExist(err):
		code = "ENOENT"
	case os.IsPermission(err):
		code = "EACCES"
	}
	if code == "" {
		return err
	}
	return &harnessenv.FileError{Code: code, Path: path, Err: err}
}

func compatibilityNotSupported(operation string) error {
	return &harnessenv.FileError{
		Code: harnessenv.FileErrorNotSupported,
		Err:  fmt.Errorf("%s is not supported", operation),
	}
}
