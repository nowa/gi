//go:build windows

package gicodingagent

import "os"

func rpcProcessSignalName(_ *os.ProcessState) string {
	return ""
}
