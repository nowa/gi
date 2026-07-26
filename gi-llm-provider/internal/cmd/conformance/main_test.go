package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestCommittedPiPayloadAndCostFixtures(t *testing.T) {
	root := conformanceRepositoryRoot(t)
	cases := readJSONLines[envelope](
		t,
		filepath.Join(root, "docs/pi-parity/differential/cases.jsonl"),
	)
	fixtures := readJSONLines[map[string]any](
		t,
		filepath.Join(root, "docs/pi-parity/differential/pi-v0.82.1.jsonl"),
	)
	expectedByID := make(map[string]map[string]any, len(fixtures))
	for _, fixture := range fixtures {
		id, _ := fixture["id"].(string)
		if id == "" {
			t.Fatalf("fixture without id: %#v", fixture)
		}
		if _, duplicate := expectedByID[id]; duplicate {
			t.Fatalf("duplicate fixture id %q", id)
		}
		expectedByID[id] = fixture
	}

	for _, testCase := range cases {
		expected, ok := expectedByID[testCase.ID]
		if !ok {
			t.Errorf("%s: missing committed Pi fixture", testCase.ID)
			continue
		}
		actual := normalizeJSONValue(t, execute(testCase))
		if !reflect.DeepEqual(actual, expected) {
			expectedJSON, _ := json.Marshal(expected)
			actualJSON, _ := json.Marshal(actual)
			t.Errorf("%s:\nPi: %s\nGi: %s", testCase.ID, expectedJSON, actualJSON)
		}
		delete(expectedByID, testCase.ID)
	}
	for id := range expectedByID {
		t.Errorf("%s: fixture has no input case", id)
	}
}

func conformanceRepositoryRoot(t *testing.T) string {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate conformance test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(source), "..", "..", "..", ".."))
}

func readJSONLines[T any](t *testing.T, file string) []T {
	t.Helper()
	content, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(bytes.NewReader(content))
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	var values []T
	for line := 1; scanner.Scan(); line++ {
		source := strings.TrimSpace(scanner.Text())
		if source == "" || strings.HasPrefix(source, "#") {
			continue
		}
		var value T
		if err := json.Unmarshal([]byte(source), &value); err != nil {
			t.Fatalf("%s:%d: %v", file, line, err)
		}
		values = append(values, value)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return values
}

func normalizeJSONValue(t *testing.T, value any) any {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var normalized any
	if err := json.Unmarshal(encoded, &normalized); err != nil {
		t.Fatal(err)
	}
	return normalized
}
