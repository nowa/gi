package gicodingagent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestProcessFileArgumentsWrapsTextFilesWithResolvedPaths(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(path, []byte("Hello, world!"), 0o600); err != nil {
		t.Fatal(err)
	}

	processed, err := ProcessFileArguments([]string{"note.txt"}, ProcessFileArgumentsOptions{CWD: dir})
	if err != nil {
		t.Fatal(err)
	}

	want := `<file name="` + path + `">` + "\nHello, world!\n</file>\n"
	if processed.Text != want || len(processed.Images) != 0 {
		t.Fatalf("processed = %#v, want text %q", processed, want)
	}
}

func TestProcessFileArgumentsSkipsEmptyFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	processed, err := ProcessFileArguments([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(processed.Text) != "" || len(processed.Images) != 0 {
		t.Fatalf("processed = %#v, want empty", processed)
	}
}

func TestProcessFileArgumentsImageResizeSuccessAddsDimensionNote(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "image.png")
	if err := os.WriteFile(path, mustDecodeBase64(t, tinyPNGBase64), 0o600); err != nil {
		t.Fatal(err)
	}

	var gotPart llm.ContentPart
	processed, err := ProcessFileArguments([]string{"image.png"}, ProcessFileArgumentsOptions{
		CWD: dir,
		ResizeImage: func(part llm.ContentPart, _ ImageResizeOptions) *ResizedImage {
			gotPart = part
			return &ResizedImage{
				Data:           "resized-image",
				MIMEType:       "image/png",
				OriginalWidth:  100,
				OriginalHeight: 50,
				Width:          50,
				Height:         25,
				WasResized:     true,
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	wantText := `<file name="` + path + `">[Image: original 100x50, displayed at 50x25. Multiply coordinates by 2.00 to map to original image.]</file>` + "\n"
	if processed.Text != wantText {
		t.Fatalf("text = %q, want %q", processed.Text, wantText)
	}
	if len(processed.Images) != 1 || processed.Images[0].Type != llm.ContentImage || processed.Images[0].Data != "resized-image" || processed.Images[0].MIMEType != "image/png" {
		t.Fatalf("images = %#v", processed.Images)
	}
	if gotPart.Type != llm.ContentImage || gotPart.Data != tinyPNGBase64 || gotPart.MIMEType != "image/png" {
		t.Fatalf("resize input = %#v", gotPart)
	}
}

func TestProcessFileArgumentsImageAutoResizeDisabledAttachesOriginal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "image.png")
	if err := os.WriteFile(path, mustDecodeBase64(t, tinyPNGBase64), 0o600); err != nil {
		t.Fatal(err)
	}

	autoResize := false
	processed, err := ProcessFileArguments([]string{path}, ProcessFileArgumentsOptions{AutoResizeImages: &autoResize})
	if err != nil {
		t.Fatal(err)
	}

	wantText := `<file name="` + path + `"></file>` + "\n"
	if processed.Text != wantText {
		t.Fatalf("text = %q, want %q", processed.Text, wantText)
	}
	if len(processed.Images) != 1 || processed.Images[0].Type != llm.ContentImage || processed.Images[0].Data != tinyPNGBase64 || processed.Images[0].MIMEType != "image/png" {
		t.Fatalf("images = %#v", processed.Images)
	}
}
