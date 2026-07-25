package gicodingagent

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

const (
	sessionReadBufferSize     = 1024 * 1024
	maxSessionHeaderScanBytes = 1024 * 1024
)

// SessionHeaderScanLimitError identifies a file whose first parsed entry could
// not be discovered within the bounded header scan. Callers opening an explicit
// file may fall back to a full streaming load; directory discovery should skip
// the file.
type SessionHeaderScanLimitError struct {
	Path  string
	Limit int
}

func (e *SessionHeaderScanLimitError) Error() string {
	if e == nil {
		return "session header scan limit exceeded"
	}
	return fmt.Sprintf("session header exceeds %d-byte scan limit: %s", e.Limit, e.Path)
}

func parseSessionEntryLine(line []byte) (FileEntry, bool) {
	if len(bytes.TrimSpace(line)) == 0 {
		return FileEntry{}, false
	}
	var entry FileEntry
	if err := json.Unmarshal(line, &entry); err != nil {
		return FileEntry{}, false
	}
	return entry, true
}

func sessionEntryHasStringID(entry FileEntry) bool {
	if entry.raw != nil {
		_, ok := entry.raw["id"].(string)
		return ok
	}
	return entry.ID != ""
}

// loadEntriesFromFile is the authoritative reader. It does not impose a line
// limit because explicit session opens must continue to support legacy files
// with very large headers or malformed prefixes.
func loadEntriesFromFile(filePath string) ([]FileEntry, error) {
	file, err := os.Open(filePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := bufio.NewReaderSize(file, sessionReadBufferSize)
	var entries []FileEntry
	for {
		line, readErr := reader.ReadBytes('\n')
		if entry, ok := parseSessionEntryLine(line); ok {
			entries = append(entries, entry)
		}
		switch {
		case readErr == nil:
			continue
		case errors.Is(readErr, io.EOF):
			if len(entries) == 0 {
				return entries, nil
			}
			if entries[0].Type != "session" || !sessionEntryHasStringID(entries[0]) {
				return nil, nil
			}
			return entries, nil
		default:
			return nil, readErr
		}
	}
}

// LoadEntriesFromFile loads valid JSONL records and returns no entries when the
// first parsed record is not a session header.
func LoadEntriesFromFile(filePath string) []FileEntry {
	entries, _ := loadEntriesFromFile(filePath)
	return entries
}

// parseSessionHeaderCandidate mirrors the authoritative loader: blank and
// malformed physical lines are skipped, while the first parsed non-header
// record makes the file invalid. decided distinguishes "keep scanning" from a
// conclusive nil header.
func parseSessionHeaderCandidate(line []byte) (header *SessionHeader, decided bool) {
	entry, ok := parseSessionEntryLine(line)
	if !ok {
		return nil, false
	}
	if entry.Type != "session" || !sessionEntryHasStringID(entry) {
		return nil, true
	}
	return &SessionHeader{
		Type:          entry.Type,
		Version:       entry.Version,
		ID:            entry.ID,
		Timestamp:     entry.Timestamp,
		CWD:           entry.CWD,
		ParentSession: entry.ParentSession,
	}, true
}

func readSessionHeader(filePath string) (*SessionHeader, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Read one byte beyond the limit so an entry ending exactly at the boundary
	// remains valid while an entry requiring more input is distinguishable.
	content, err := io.ReadAll(io.LimitReader(file, maxSessionHeaderScanBytes+1))
	if err != nil {
		return nil, err
	}
	exceedsLimit := len(content) > maxSessionHeaderScanBytes
	scanned := content
	if exceedsLimit {
		scanned = content[:maxSessionHeaderScanBytes]
	}

	lineStart := 0
	for {
		newline := bytes.IndexByte(scanned[lineStart:], '\n')
		if newline < 0 {
			break
		}
		lineEnd := lineStart + newline
		if header, decided := parseSessionHeaderCandidate(scanned[lineStart:lineEnd]); decided {
			return header, nil
		}
		lineStart = lineEnd + 1
	}
	if !exceedsLimit {
		if header, decided := parseSessionHeaderCandidate(scanned[lineStart:]); decided {
			return header, nil
		}
		return nil, nil
	}
	return nil, &SessionHeaderScanLimitError{
		Path:  filePath,
		Limit: maxSessionHeaderScanBytes,
	}
}

func readSessionHeaderForDiscovery(filePath string) *SessionHeader {
	header, err := readSessionHeader(filePath)
	if err != nil {
		return nil
	}
	return header
}

func writeSessionEntries(filePath string, entries []FileEntry, flags int) error {
	// Validate every record before opening with O_TRUNC so an unsupported custom
	// value cannot destroy an otherwise valid session.
	for _, entry := range entries {
		if _, err := json.Marshal(entry); err != nil {
			return err
		}
	}
	file, err := os.OpenFile(filePath, flags, 0o644)
	if err != nil {
		return err
	}
	writer := bufio.NewWriter(file)
	var writeErr error
	for _, entry := range entries {
		line, marshalErr := json.Marshal(entry)
		if marshalErr != nil {
			writeErr = marshalErr
			break
		}
		if _, writeErr = writer.Write(line); writeErr != nil {
			break
		}
		if writeErr = writer.WriteByte('\n'); writeErr != nil {
			break
		}
	}
	writeErr = errors.Join(writeErr, writer.Flush())
	closeErr := file.Close()
	err = errors.Join(writeErr, closeErr)
	if err != nil && flags&os.O_EXCL != 0 {
		_ = os.Remove(filePath)
	}
	return err
}
