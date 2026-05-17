package harness

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type InMemorySessionRepo struct {
	sessions map[string]*Session
}

func NewInMemorySessionRepo() *InMemorySessionRepo {
	return &InMemorySessionRepo{sessions: map[string]*Session{}}
}

func (r *InMemorySessionRepo) Create(id string) (*Session, error) {
	if id == "" {
		id = UUIDv7()
	}
	storage, err := NewInMemorySessionStorage(&SessionMetadata{ID: id, CreatedAt: nowISO()}, nil)
	if err != nil {
		return nil, err
	}
	session := NewSession(storage)
	r.sessions[id] = session
	return session, nil
}

func (r *InMemorySessionRepo) Open(metadata SessionMetadata) (*Session, error) {
	session := r.sessions[metadata.ID]
	if session == nil {
		return nil, newSessionError("not_found", "Session not found: %s", metadata.ID)
	}
	return session, nil
}

func (r *InMemorySessionRepo) List() []SessionMetadata {
	result := make([]SessionMetadata, 0, len(r.sessions))
	for _, session := range r.sessions {
		result = append(result, session.Metadata())
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (r *InMemorySessionRepo) Delete(metadata SessionMetadata) {
	delete(r.sessions, metadata.ID)
}

func (r *InMemorySessionRepo) Fork(sourceMetadata SessionMetadata, id string, entryID *string, includeEntry bool) (*Session, error) {
	source, err := r.Open(sourceMetadata)
	if err != nil {
		return nil, err
	}
	entries, err := entriesToFork(source.Storage(), entryID, includeEntry)
	if err != nil {
		return nil, err
	}
	if id == "" {
		id = UUIDv7()
	}
	storage, err := NewInMemorySessionStorage(&SessionMetadata{ID: id, CreatedAt: nowISO()}, entries)
	if err != nil {
		return nil, err
	}
	session := NewSession(storage)
	r.sessions[id] = session
	return session, nil
}

type JsonlSessionRepo struct {
	SessionsRoot string
}

func NewJsonlSessionRepo(root string) *JsonlSessionRepo {
	return &JsonlSessionRepo{SessionsRoot: root}
}

func (r *JsonlSessionRepo) Create(cwd, id string) (*Session, error) {
	if id == "" {
		id = UUIDv7()
	}
	createdAt := nowISO()
	dir := filepath.Join(r.SessionsRoot, encodeCWD(cwd))
	path := filepath.Join(dir, strings.NewReplacer(":", "-", ".", "-").Replace(createdAt)+"_"+id+".jsonl")
	storage, err := CreateJsonlSessionStorage(path, SessionMetadata{ID: id, CreatedAt: createdAt, CWD: cwd})
	if err != nil {
		return nil, err
	}
	return NewSession(storage), nil
}

func (r *JsonlSessionRepo) Open(metadata SessionMetadata) (*Session, error) {
	if _, err := os.Stat(metadata.Path); err != nil {
		return nil, newSessionError("not_found", "Session not found: %s", metadata.Path)
	}
	storage, err := OpenJsonlSessionStorage(metadata.Path)
	if err != nil {
		return nil, err
	}
	return NewSession(storage), nil
}

func (r *JsonlSessionRepo) List(cwd string) ([]SessionMetadata, error) {
	var dirs []string
	if cwd != "" {
		dirs = []string{filepath.Join(r.SessionsRoot, encodeCWD(cwd))}
	} else {
		entries, err := os.ReadDir(r.SessionsRoot)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, nil
			}
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() {
				dirs = append(dirs, filepath.Join(r.SessionsRoot, entry.Name()))
			}
		}
	}
	var result []SessionMetadata
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
				continue
			}
			metadata, err := LoadJsonlSessionMetadata(filepath.Join(dir, entry.Name()))
			if err == nil {
				result = append(result, metadata)
			}
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt > result[j].CreatedAt })
	return result, nil
}

func (r *JsonlSessionRepo) Delete(metadata SessionMetadata) error {
	return os.Remove(metadata.Path)
}

func (r *JsonlSessionRepo) Fork(sourceMetadata SessionMetadata, cwd, id string, entryID *string, includeEntry bool) (*Session, error) {
	source, err := r.Open(sourceMetadata)
	if err != nil {
		return nil, err
	}
	entries, err := entriesToFork(source.Storage(), entryID, includeEntry)
	if err != nil {
		return nil, err
	}
	if id == "" {
		id = UUIDv7()
	}
	createdAt := nowISO()
	dir := filepath.Join(r.SessionsRoot, encodeCWD(cwd))
	path := filepath.Join(dir, strings.NewReplacer(":", "-", ".", "-").Replace(createdAt)+"_"+id+".jsonl")
	storage, err := CreateJsonlSessionStorage(path, SessionMetadata{ID: id, CreatedAt: createdAt, CWD: cwd, ParentSessionPath: sourceMetadata.Path})
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if err := storage.AppendEntry(entry); err != nil {
			return nil, err
		}
	}
	return NewSession(storage), nil
}

func entriesToFork(storage SessionStorage, entryID *string, includeEntry bool) ([]Entry, error) {
	leaf := entryID
	if leaf == nil {
		current, ok, err := storage.LeafID()
		if err != nil {
			return nil, err
		}
		if ok {
			leaf = &current
		}
	}
	path, err := storage.PathToRoot(leaf)
	if err != nil {
		return nil, err
	}
	if entryID != nil && !includeEntry && len(path) > 0 {
		path = path[:len(path)-1]
	}
	return path, nil
}

func encodeCWD(cwd string) string {
	trimmed := strings.TrimLeft(cwd, `/\`)
	replacer := strings.NewReplacer("/", "-", "\\", "-", ":", "-")
	return "--" + replacer.Replace(trimmed) + "--"
}
