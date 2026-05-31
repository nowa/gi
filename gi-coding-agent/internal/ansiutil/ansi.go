package ansiutil

import (
	"regexp"
	"strings"
)

var oscStripPattern = regexp.MustCompile(`\x1b\][\s\S]*?(?:\x07|\x1b\\|\x{9c})`)

var csiStripPattern = func() *regexp.Regexp {
	re := regexp.MustCompile(`[\x1b\x{9b}][\[\]()#;?]*(?:[0-9]{1,4}(?:[;:][0-9]{0,4})*)?[0-9A-PR-TZcf-nq-uy=><~]`)
	re.Longest()
	return re
}()

func Strip(value string) string {
	if !strings.Contains(value, "\x1b") && !strings.Contains(value, "\x9b") {
		return value
	}
	value = oscStripPattern.ReplaceAllString(value, "")
	return csiStripPattern.ReplaceAllString(value, "")
}
