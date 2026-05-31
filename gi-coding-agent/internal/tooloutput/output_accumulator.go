package tooloutput

import (
	"bytes"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"

	agentharness "github.com/nowa/gi/gi-agent-core/harness"
)

const DefaultLineLimit = 2000

var ansiOSCStripPattern = regexp.MustCompile(`\x1b\][\s\S]*?(?:\x07|\x1b\\|\x{9c})`)

var ansiCSIStripPattern = func() *regexp.Regexp {
	re := regexp.MustCompile(`[\x1b\x{9b}][\[\]()#;?]*(?:[0-9]{1,4}(?:[;:][0-9]{0,4})*)?[0-9A-PR-TZcf-nq-uy=><~]`)
	re.Longest()
	return re
}()

type AccumulatorOptions struct {
	MaxLines       int
	MaxBytes       int
	TempFilePrefix string
}

type Snapshot struct {
	Content        string
	Truncation     agentharness.TruncationResult
	FullOutputPath string
	LastLineBytes  int
}

type Accumulator struct {
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

func NewAccumulator(options AccumulatorOptions) *Accumulator {
	maxLines := options.MaxLines
	if maxLines <= 0 {
		maxLines = DefaultLineLimit
	}
	maxBytes := options.MaxBytes
	if maxBytes <= 0 {
		maxBytes = agentharness.DefaultMaxBytes
	}
	prefix := options.TempFilePrefix
	if prefix == "" {
		prefix = "gi-bash-output"
	}
	return &Accumulator{
		maxLines:                 maxLines,
		maxBytes:                 maxBytes,
		maxRollingBytes:          max(maxBytes*2, 1),
		tempFilePrefix:           prefix,
		tailStartsAtLineBoundary: true,
		totalLines:               1,
	}
}

func (a *Accumulator) Append(data []byte) {
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

func (a *Accumulator) Snapshot(persistIfTruncated bool) Snapshot {
	text := SanitizeBytes(a.snapshotRaw())
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
	return Snapshot{
		Content:        truncation.Content,
		Truncation:     truncation,
		FullOutputPath: a.fullOutputPath(),
		LastLineBytes:  a.currentLineBytes,
	}
}

func (a *Accumulator) Close() {
	if a.tempFile == nil {
		return
	}
	if err := a.tempFile.Close(); err != nil && a.fileErr == nil {
		a.fileErr = err
	}
	a.tempFile = nil
}

func (a *Accumulator) TotalRawBytes() int {
	return a.totalRawBytes
}

func (a *Accumulator) RawBufferedBytes() int {
	return len(a.rawChunks)
}

func (a *Accumulator) appendTail(data []byte) {
	a.tailRaw = append(a.tailRaw, data...)
	a.tailBytes += len(data)
	if a.tailBytes <= a.maxRollingBytes*2 {
		return
	}
	a.trimTail()
}

func (a *Accumulator) trimTail() {
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

func (a *Accumulator) snapshotRaw() []byte {
	if a.tailStartsAtLineBoundary {
		return append([]byte(nil), a.tailRaw...)
	}
	if index := bytes.IndexByte(a.tailRaw, '\n'); index >= 0 {
		return append([]byte(nil), a.tailRaw[index+1:]...)
	}
	return append([]byte(nil), a.tailRaw...)
}

func (a *Accumulator) countLines(data []byte) {
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

func (a *Accumulator) shouldUseTempFile() bool {
	return a.totalRawBytes > a.maxBytes || a.totalLines > a.maxLines
}

func (a *Accumulator) ensureTempFile() {
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

func (a *Accumulator) fullOutputPath() string {
	if a.fileErr != nil {
		return ""
	}
	return a.tempFilePath
}

func SanitizeBytes(data []byte) string {
	text := string(bytes.ToValidUTF8(data, []byte{}))
	text = stripANSI(text)
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return text
}

func stripANSI(text string) string {
	if !strings.Contains(text, "\x1b") && !strings.Contains(text, "\x9b") {
		return text
	}
	text = ansiOSCStripPattern.ReplaceAllString(text, "")
	return ansiCSIStripPattern.ReplaceAllString(text, "")
}
