package gicodingagent

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
)

const tinyPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8DwHwAFBQIAX8jx0gAAAABJRU5ErkJggg=="

func TestSettingsManagerBlockImagesPiMatrix(t *testing.T) {
	manager := NewInMemorySettingsManager(map[string]any{})
	if manager.GetBlockImages() {
		t.Fatal("blockImages default = true, want false")
	}

	manager = NewInMemorySettingsManager(map[string]any{"images": map[string]any{"blockImages": true}})
	if !manager.GetBlockImages() {
		t.Fatal("blockImages = false, want true")
	}

	manager = NewInMemorySettingsManager(map[string]any{})
	if manager.GetBlockImages() {
		t.Fatal("initial blockImages = true, want false")
	}
	manager.SetBlockImages(true)
	if !manager.GetBlockImages() {
		t.Fatal("blockImages after set true = false")
	}
	manager.SetBlockImages(false)
	if manager.GetBlockImages() {
		t.Fatal("blockImages after set false = true")
	}

	manager = NewInMemorySettingsManager(map[string]any{"images": map[string]any{"autoResize": true, "blockImages": true}})
	if !manager.GetImageAutoResize() || !manager.GetBlockImages() {
		t.Fatalf("image settings: autoResize=%v blockImages=%v", manager.GetImageAutoResize(), manager.GetBlockImages())
	}
}

func TestReadToolAlwaysReadsImagesAndText(t *testing.T) {
	dir := t.TempDir()
	imagePath := filepath.Join(dir, "test.png")
	if err := os.WriteFile(imagePath, mustDecodeBase64(t, tinyPNGBase64), 0o600); err != nil {
		t.Fatal(err)
	}
	textPath := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(textPath, []byte("Hello, world!"), 0o600); err != nil {
		t.Fatal(err)
	}

	tool := NewReadTool(dir)
	imageResult, err := tool.Execute("test-1", ReadToolInput{Path: imagePath})
	if err != nil {
		t.Fatal(err)
	}
	if len(imageResult.Content) < 1 || !hasContentPartType(imageResult.Content, llm.ContentImage) {
		t.Fatalf("image read content = %#v", imageResult.Content)
	}

	textResult, err := tool.Execute("test-2", ReadToolInput{Path: textPath})
	if err != nil {
		t.Fatal(err)
	}
	if len(textResult.Content) != 1 || textResult.Content[0].Type != llm.ContentText || !strings.Contains(textResult.Content[0].Text, "Hello, world!") {
		t.Fatalf("text read content = %#v", textResult.Content)
	}
}

func TestProcessFileArgumentsAlwaysProcessesImagesAndText(t *testing.T) {
	dir := t.TempDir()
	imagePath := filepath.Join(dir, "test.png")
	if err := os.WriteFile(imagePath, mustDecodeBase64(t, tinyPNGBase64), 0o600); err != nil {
		t.Fatal(err)
	}
	textPath := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(textPath, []byte("Hello, world!"), 0o600); err != nil {
		t.Fatal(err)
	}

	imageResult, err := ProcessFileArguments([]string{imagePath})
	if err != nil {
		t.Fatal(err)
	}
	if len(imageResult.Images) != 1 || imageResult.Images[0].Type != llm.ContentImage {
		t.Fatalf("processed image result = %#v", imageResult)
	}

	textResult, err := ProcessFileArguments([]string{textPath})
	if err != nil {
		t.Fatal(err)
	}
	if len(textResult.Images) != 0 || !strings.Contains(textResult.Text, "Hello, world!") {
		t.Fatalf("processed text result = %#v", textResult)
	}
}

func hasContentPartType(parts []llm.ContentPart, partType string) bool {
	for _, part := range parts {
		if part.Type == partType {
			return true
		}
	}
	return false
}

func mustDecodeBase64(t *testing.T, value string) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
