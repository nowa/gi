package gicodingagent

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

func SerializeJSONLine(record any) (string, error) {
	data, err := json.Marshal(record)
	if err != nil {
		return "", err
	}
	line := string(data)
	line = strings.ReplaceAll(line, `\u2028`, "\u2028")
	line = strings.ReplaceAll(line, `\u2029`, "\u2029")
	line = strings.ReplaceAll(line, `\U2028`, "\u2028")
	line = strings.ReplaceAll(line, `\U2029`, "\u2029")
	return line + "\n", nil
}

func AttachJSONLLineReader(reader io.Reader, onLine func(string)) error {
	if onLine == nil {
		_, err := io.Copy(io.Discard, reader)
		return err
	}
	buffered := bufio.NewReader(reader)
	for {
		line, err := buffered.ReadBytes('\n')
		if len(line) > 0 {
			line = bytes.TrimSuffix(line, []byte{'\n'})
			line = bytes.TrimSuffix(line, []byte{'\r'})
			if len(line) > 0 {
				onLine(string(line))
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}
