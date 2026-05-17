package harness

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestInMemorySessionRepo(t *testing.T) {
	repo := NewInMemorySessionRepo()
	session, err := repo.Create("session-1")
	if err != nil {
		t.Fatal(err)
	}
	metadata := session.Metadata()
	user1, _ := session.AppendMessage(harnessUserMessage("one"))
	assistant1, _ := session.AppendMessage(harnessAssistantMessage("two"))
	user2, _ := session.AppendMessage(harnessUserMessage("three"))
	opened, err := repo.Open(metadata)
	if err != nil || opened != session {
		t.Fatalf("open = %v %v", opened, err)
	}
	if got := metadataIDs(repo.List()); !reflect.DeepEqual(got, []string{"session-1"}) {
		t.Fatalf("list = %#v", got)
	}
	fork, err := repo.Fork(metadata, "session-2", &user2, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := entryIDs(fork.Entries()); !reflect.DeepEqual(got, []string{user1, assistant1}) {
		t.Fatalf("fork entries = %#v", got)
	}
	fullFork, err := repo.Fork(metadata, "session-3", nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := entryIDs(fullFork.Entries()); !reflect.DeepEqual(got, []string{user1, assistant1, user2}) {
		t.Fatalf("full fork entries = %#v", got)
	}
	repo.Delete(metadata)
	if _, err := repo.Open(metadata); err == nil || !strings.Contains(err.Error(), "Session not found: session-1") {
		t.Fatalf("open deleted err = %v", err)
	}
}

func TestJsonlSessionRepoStoresByEncodedCWDAndForks(t *testing.T) {
	root := t.TempDir()
	repo := NewJsonlSessionRepo(root)
	cwd := "/tmp/my-project"
	otherCwd := "/tmp/other-project"
	session, err := repo.Create(cwd, "019de8c2-de29-73e9-ae0c-e134db34c447")
	if err != nil {
		t.Fatal(err)
	}
	other, err := repo.Create(otherCwd, "other-session")
	if err != nil {
		t.Fatal(err)
	}
	metadata := session.Metadata()
	otherMetadata := other.Metadata()
	if !strings.Contains(metadata.Path, "--tmp-my-project--") || !strings.Contains(otherMetadata.Path, "--tmp-other-project--") {
		t.Fatalf("paths = %s / %s", metadata.Path, otherMetadata.Path)
	}
	if _, err := os.Stat(metadata.Path); err != nil {
		t.Fatal(err)
	}
	listByCWD, err := repo.List(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if got := metadataIDs(listByCWD); !reflect.DeepEqual(got, []string{metadata.ID}) {
		t.Fatalf("list cwd = %#v", got)
	}
	all, err := repo.List("")
	if err != nil {
		t.Fatal(err)
	}
	gotAll := metadataIDs(all)
	sort.Strings(gotAll)
	wantAll := []string{metadata.ID, otherMetadata.ID}
	sort.Strings(wantAll)
	if !reflect.DeepEqual(gotAll, wantAll) {
		t.Fatalf("list all = %#v", gotAll)
	}

	source, err := repo.Create("/tmp/source", "source-session")
	if err != nil {
		t.Fatal(err)
	}
	sourceMetadata := source.Metadata()
	user1, _ := source.AppendMessage(harnessUserMessage("one"))
	assistant1, _ := source.AppendMessage(harnessAssistantMessage("two"))
	user2, _ := source.AppendMessage(harnessUserMessage("three"))
	opened, err := repo.Open(sourceMetadata)
	if err != nil || opened.Metadata() != sourceMetadata {
		t.Fatalf("open source = %#v %v", opened.Metadata(), err)
	}
	fork, err := repo.Fork(sourceMetadata, "/tmp/target", "fork-session", &user2, false)
	if err != nil {
		t.Fatal(err)
	}
	forkMetadata := fork.Metadata()
	if forkMetadata.CWD != "/tmp/target" || forkMetadata.ParentSessionPath != sourceMetadata.Path {
		t.Fatalf("fork metadata = %#v", forkMetadata)
	}
	if got := entryIDs(fork.Entries()); !reflect.DeepEqual(got, []string{user1, assistant1}) {
		t.Fatalf("fork entries = %#v", got)
	}
	fullFork, err := repo.Fork(sourceMetadata, "/tmp/target", "full-fork-session", nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := entryIDs(fullFork.Entries()); !reflect.DeepEqual(got, []string{user1, assistant1, user2}) {
		t.Fatalf("full fork entries = %#v", got)
	}
	if err := repo.Delete(sourceMetadata); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sourceMetadata.Path); !os.IsNotExist(err) {
		t.Fatalf("source file still exists: %v", err)
	}
	if _, err := repo.Open(sourceMetadata); err == nil {
		t.Fatal("expected open deleted session to fail")
	}

	_ = filepath.Separator
}

func metadataIDs(metadata []SessionMetadata) []string {
	ids := make([]string, len(metadata))
	for i, item := range metadata {
		ids[i] = item.ID
	}
	return ids
}
