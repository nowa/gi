package utils

import (
	"reflect"
	"testing"
)

func TestSplitLinesForCountingMatchesTruncationContract(t *testing.T) {
	for _, test := range []struct {
		name    string
		content string
		want    []string
	}{
		{name: "empty", content: "", want: nil},
		{name: "one line", content: "one", want: []string{"one"}},
		{name: "trailing newline", content: "one\n", want: []string{"one"}},
		{name: "interior empty line", content: "one\n\ntwo\n", want: []string{"one", "", "two"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := splitLinesForCounting(test.content); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("lines = %#v, want %#v", got, test.want)
			}
		})
	}
}
