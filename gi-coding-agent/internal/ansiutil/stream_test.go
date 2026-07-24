package ansiutil

import "testing"

func TestStreamStripperPreservesUTF8AcrossWrites(t *testing.T) {
	var stripper StreamStripper
	euro := []byte("€")
	output := append([]byte(nil), stripper.Write(euro[:1])...)
	output = append(output, stripper.Write(euro[1:])...)
	if got := string(output); got != "€" {
		t.Fatalf("output = %q, want euro sign", got)
	}
}

func TestStreamStripperRemovesSplitANSISequences(t *testing.T) {
	var stripper StreamStripper
	var output []byte
	for _, chunk := range [][]byte{
		[]byte("before "),
		[]byte("\x1b[3"),
		[]byte("1mred"),
		[]byte("\x1b[0"),
		[]byte("m after "),
		[]byte("\x1b]0;ignored"),
		[]byte("\x1b\\done"),
	} {
		output = append(output, stripper.Write(chunk)...)
	}
	if got, want := string(output), "before red after done"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}
