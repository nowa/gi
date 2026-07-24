package gicodingagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	llm "github.com/nowa/gi/gi-llm-provider"
)

// FileModelsStore persists dynamic provider catalogs in one locked JSON file.
// Each mutation reads and rewrites the complete map while holding the
// path-scoped lock so unrelated provider entries cannot be lost.
type FileModelsStore struct {
	path string
}

var _ llm.ModelsStore = (*FileModelsStore)(nil)

// NewFileModelsStore creates a JSON-backed dynamic catalog store.
func NewFileModelsStore(path string) *FileModelsStore {
	return &FileModelsStore{path: path}
}

func (s *FileModelsStore) ReadModels(
	ctx context.Context,
	providerID string,
) (llm.ModelsStoreEntry, bool, error) {
	if err := modelStoreContextError(ctx); err != nil {
		return llm.ModelsStoreEntry{}, false, err
	}
	if s == nil || s.path == "" {
		return llm.ModelsStoreEntry{}, false, errors.New(
			"models store path is required",
		)
	}

	lock := authFileMutex(s.path)
	lock.Lock()
	defer lock.Unlock()
	if err := modelStoreContextError(ctx); err != nil {
		return llm.ModelsStoreEntry{}, false, err
	}
	entries, err := readModelsStoreFile(s.path)
	if err != nil {
		return llm.ModelsStoreEntry{}, false, err
	}
	entry, ok := entries[providerID]
	return entry, ok, nil
}

func (s *FileModelsStore) WriteModels(
	ctx context.Context,
	providerID string,
	entry llm.ModelsStoreEntry,
) error {
	if err := modelStoreContextError(ctx); err != nil {
		return err
	}
	if s == nil || s.path == "" {
		return errors.New("models store path is required")
	}

	lock := authFileMutex(s.path)
	lock.Lock()
	defer lock.Unlock()
	if err := modelStoreContextError(ctx); err != nil {
		return err
	}
	entries, err := readModelsStoreFile(s.path)
	if err != nil {
		return err
	}
	entries[providerID] = entry
	return writeModelsStoreFile(ctx, s.path, entries)
}

func (s *FileModelsStore) DeleteModels(
	ctx context.Context,
	providerID string,
) error {
	if err := modelStoreContextError(ctx); err != nil {
		return err
	}
	if s == nil || s.path == "" {
		return errors.New("models store path is required")
	}

	lock := authFileMutex(s.path)
	lock.Lock()
	defer lock.Unlock()
	if err := modelStoreContextError(ctx); err != nil {
		return err
	}
	entries, err := readModelsStoreFile(s.path)
	if err != nil {
		return err
	}
	if _, ok := entries[providerID]; !ok {
		return nil
	}
	delete(entries, providerID)
	return writeModelsStoreFile(ctx, s.path, entries)
}

func readModelsStoreFile(path string) (map[string]llm.ModelsStoreEntry, error) {
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]llm.ModelsStoreEntry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read models store: %w", err)
	}
	if len(content) == 0 {
		return map[string]llm.ModelsStoreEntry{}, nil
	}
	var entries map[string]llm.ModelsStoreEntry
	if err := json.Unmarshal(content, &entries); err != nil {
		return nil, fmt.Errorf("parse models store: %w", err)
	}
	if entries == nil {
		entries = map[string]llm.ModelsStoreEntry{}
	}
	return entries, nil
}

func writeModelsStoreFile(
	ctx context.Context,
	path string,
	entries map[string]llm.ModelsStoreEntry,
) error {
	if err := modelStoreContextError(ctx); err != nil {
		return err
	}
	content, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("encode models store: %w", err)
	}
	content = append(content, '\n')

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create models store directory: %w", err)
	}
	temp, err := os.CreateTemp(dir, ".models-store-*.tmp")
	if err != nil {
		return fmt.Errorf("create models store temporary file: %w", err)
	}
	tempPath := temp.Name()
	defer func() {
		_ = temp.Close()
		_ = os.Remove(tempPath)
	}()

	if err := temp.Chmod(0o600); err != nil {
		return fmt.Errorf("set models store permissions: %w", err)
	}
	if _, err := temp.Write(content); err != nil {
		return fmt.Errorf("write models store: %w", err)
	}
	if err := temp.Sync(); err != nil {
		return fmt.Errorf("sync models store: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close models store: %w", err)
	}
	if err := modelStoreContextError(ctx); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace models store: %w", err)
	}
	return nil
}

func modelStoreContextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
