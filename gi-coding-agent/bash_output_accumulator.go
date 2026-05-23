package gicodingagent

import (
	"bytes"
	"os"
	"unicode/utf8"

	agentharness "github.com/nowa/gi/gi-agent-core/harness"
)

type bashOutputAccumulatorOptions struct {
	MaxLines       int
	MaxBytes       int
	TempFilePrefix string
}

type bashOutputSnapshot struct {
	Content        string
	Truncation     agentharness.TruncationResult
	FullOutputPath string
	LastLineBytes  int
}

type bashOutputAccumulator struct {
	maxLines        int
	maxBytes        int
	maxRollingBytes int
	tempFilePrefix  string

	rawChunks []byte
	tailRaw   []byte
	tailBytes int

	tailStartsAtLineBoundary bool
	totalRawBytes            int
	totalLines               int
	currentLineBytes         int

	tempFilePath string
	tempFile     *os.File
	fileErr      error
}

func newBashOutputAccumulator(options bashOutputAccumulatorOptions) *bashOutputAccumulator {
	maxLines := options.MaxLines
	if maxLines <= 0 {
		maxLines = defaultBashOutputLineLimit
	}
	maxBytes := options.MaxBytes
	if maxBytes <= 0 {
		maxBytes = agentharness.DefaultMaxBytes
	}
	prefix := options.TempFilePrefix
	if prefix == "" {
		prefix = "gi-bash-output"
	}
	return &bashOutputAccumulator{
		maxLines:                 maxLines,
		maxBytes:                 maxBytes,
		maxRollingBytes:          max(maxBytes*2, 1),
		tempFilePrefix:           prefix,
		tailStartsAtLineBoundary: true,
		totalLines:               1,
	}
}

func (a *bashOutputAccumulator) Append(data []byte) {
	if len(data) == 0 {
		return
	}
	copied := append([]byte(nil), data...)
	a.totalRawBytes += len(copied)
	a.appendTail(copied)
	a.countLines(copied)

	if a.tempFile != nil || a.shouldUseTempFile() {
		a.ensureTempFile()
		if a.tempFile != nil && a.fileErr == nil {
			if _, err := a.tempFile.Write(copied); err != nil {
				a.fileErr = err
			}
		}
		return
	}
	a.rawChunks = append(a.rawChunks, copied...)
}

func (a *bashOutputAccumulator) Snapshot(persistIfTruncated bool) bashOutputSnapshot {
	text := sanitizeBashOutputBytes(a.snapshotRaw())
	truncation := agentharness.TruncateTail(text, agentharness.TruncationOptions{
		MaxLines: a.maxLines,
		MaxBytes: a.maxBytes,
	})
	truncated := a.totalLines > a.maxLines || a.totalRawBytes > a.maxBytes || truncation.Truncated
	if truncated {
		truncation.Truncated = true
		if truncation.TruncatedBy == "" {
			if a.totalRawBytes > a.maxBytes {
				truncation.TruncatedBy = agentharness.TruncatedByBytes
			} else {
				truncation.TruncatedBy = agentharness.TruncatedByLines
			}
		}
		truncation.TotalLines = a.totalLines
		truncation.TotalBytes = a.totalRawBytes
		truncation.MaxLines = a.maxLines
		truncation.MaxBytes = a.maxBytes
	}
	if persistIfTruncated && truncated {
		a.ensureTempFile()
	}
	return bashOutputSnapshot{
		Content:        truncation.Content,
		Truncation:     truncation,
		FullOutputPath: a.fullOutputPath(),
		LastLineBytes:  a.currentLineBytes,
	}
}

func (a *bashOutputAccumulator) Close() {
	if a.tempFile == nil {
		return
	}
	if err := a.tempFile.Close(); err != nil && a.fileErr == nil {
		a.fileErr = err
	}
	a.tempFile = nil
}

func (a *bashOutputAccumulator) appendTail(data []byte) {
	a.tailRaw = append(a.tailRaw, data...)
	a.tailBytes += len(data)
	if a.tailBytes <= a.maxRollingBytes*2 {
		return
	}
	a.trimTail()
}

func (a *bashOutputAccumulator) trimTail() {
	if len(a.tailRaw) <= a.maxRollingBytes {
		a.tailBytes = len(a.tailRaw)
		return
	}
	start := len(a.tailRaw) - a.maxRollingBytes
	for start < len(a.tailRaw) && !utf8.RuneStart(a.tailRaw[start]) {
		start++
	}
	if start >= len(a.tailRaw) {
		a.tailRaw = nil
		a.tailBytes = 0
		a.tailStartsAtLineBoundary = false
		return
	}
	if start > 0 {
		previous := a.tailRaw[start-1]
		a.tailStartsAtLineBoundary = previous == '\n' || previous == '\r'
	}
	a.tailRaw = append([]byte(nil), a.tailRaw[start:]...)
	a.tailBytes = len(a.tailRaw)
}

func (a *bashOutputAccumulator) snapshotRaw() []byte {
	if a.tailStartsAtLineBoundary {
		return append([]byte(nil), a.tailRaw...)
	}
	if index := bytes.IndexByte(a.tailRaw, '\n'); index >= 0 {
		return append([]byte(nil), a.tailRaw[index+1:]...)
	}
	return append([]byte(nil), a.tailRaw...)
}

func (a *bashOutputAccumulator) countLines(data []byte) {
	normalized := bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	normalized = bytes.ReplaceAll(normalized, []byte("\r"), []byte("\n"))
	if len(normalized) == 0 {
		return
	}
	if lastNewline := bytes.LastIndexByte(normalized, '\n'); lastNewline >= 0 {
		a.totalLines += bytes.Count(normalized, []byte{'\n'})
		a.currentLineBytes = len(normalized[lastNewline+1:])
		return
	}
	a.currentLineBytes += len(normalized)
}

func (a *bashOutputAccumulator) shouldUseTempFile() bool {
	return a.totalRawBytes > a.maxBytes || a.totalLines > a.maxLines
}

func (a *bashOutputAccumulator) ensureTempFile() {
	if a.tempFile != nil || a.tempFilePath != "" {
		return
	}
	file, err := os.CreateTemp("", a.tempFilePrefix+"-*.txt")
	if err != nil {
		a.fileErr = err
		return
	}
	a.tempFile = file
	a.tempFilePath = file.Name()
	if len(a.rawChunks) > 0 {
		if _, err := a.tempFile.Write(a.rawChunks); err != nil {
			a.fileErr = err
		}
		a.rawChunks = nil
	}
}

func (a *bashOutputAccumulator) fullOutputPath() string {
	if a.fileErr != nil {
		return ""
	}
	return a.tempFilePath
}
