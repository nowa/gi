package gicodingagent

import (
	"io"

	"github.com/nowa/gi/gi-coding-agent/internal/rpcwire"
)

func SerializeJSONLine(record any) (string, error) {
	return rpcwire.SerializeJSONLine(record)
}

func AttachJSONLLineReader(reader io.Reader, onLine func(string)) error {
	return rpcwire.AttachJSONLLineReader(reader, onLine)
}
