package gillmprovider

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestCredentialJSONPreservesProviderMetadata(t *testing.T) {
	input := Credential{
		Type:    CredentialTypeOAuth,
		Access:  "access",
		Refresh: "refresh",
		Expires: 42,
		Metadata: map[string]any{
			"accountId": "account",
			"profile": map[string]any{
				"roles": []any{"admin", "developer"},
			},
			"access": "must-not-replace-typed-field",
		},
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(data, &object); err != nil {
		t.Fatal(err)
	}
	if object["access"] != "access" {
		t.Fatalf("reserved access field = %#v", object["access"])
	}

	var output Credential
	if err := json.Unmarshal(data, &output); err != nil {
		t.Fatal(err)
	}
	if output.Type != CredentialTypeOAuth || output.Access != "access" || output.Refresh != "refresh" || output.Expires != 42 {
		t.Fatalf("typed fields = %#v", output)
	}
	if got := output.Metadata["accountId"]; got != "account" {
		t.Fatalf("accountId metadata = %#v", got)
	}
	if _, ok := output.Metadata["access"]; ok {
		t.Fatal("reserved field leaked into metadata")
	}
}

func TestInMemoryCredentialStoreClonesState(t *testing.T) {
	store := NewInMemoryCredentialStore(map[string]Credential{
		"provider": {
			Type: CredentialTypeAPIKey,
			Key:  "original",
			Env:  ProviderEnv{"ACCOUNT_ID": "original"},
			Metadata: map[string]any{
				"nested":   map[string]any{"value": "original"},
				"attempts": int64(3),
			},
		},
	})

	read, ok, err := store.ReadCredential(context.Background(), "provider")
	if err != nil || !ok {
		t.Fatalf("read: ok=%v err=%v", ok, err)
	}
	read.Env["ACCOUNT_ID"] = "mutated"
	read.Metadata["nested"].(map[string]any)["value"] = "mutated"

	afterRead, _, err := store.ReadCredential(context.Background(), "provider")
	if err != nil {
		t.Fatal(err)
	}
	if afterRead.Env["ACCOUNT_ID"] != "original" {
		t.Fatalf("read mutation changed env: %#v", afterRead.Env)
	}
	if got := afterRead.Metadata["nested"].(map[string]any)["value"]; got != "original" {
		t.Fatalf("read mutation changed metadata: %#v", got)
	}
	if _, ok := afterRead.Metadata["attempts"].(int64); !ok {
		t.Fatalf("clone changed metadata scalar type: %T", afterRead.Metadata["attempts"])
	}

	unchanged, exists, err := store.ModifyCredential(
		context.Background(),
		"provider",
		func(_ context.Context, current Credential, exists bool) (Credential, bool, error) {
			current.Key = "discarded"
			current.Env["ACCOUNT_ID"] = "discarded"
			return current, false, nil
		},
	)
	if err != nil || !exists {
		t.Fatalf("modify without write: exists=%v err=%v", exists, err)
	}
	if unchanged.Key != "original" || unchanged.Env["ACCOUNT_ID"] != "original" {
		t.Fatalf("post-modify credential = %#v", unchanged)
	}

	written, exists, err := store.ModifyCredential(
		context.Background(),
		"provider",
		func(_ context.Context, current Credential, exists bool) (Credential, bool, error) {
			current.Key = "next"
			return current, true, nil
		},
	)
	if err != nil || !exists {
		t.Fatalf("modify with write: exists=%v err=%v", exists, err)
	}
	written.Key = "mutated-return"
	persisted, _, err := store.ReadCredential(context.Background(), "provider")
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Key != "next" {
		t.Fatalf("returned credential aliased store: %#v", persisted)
	}
}

func TestInMemoryCredentialStoreListsOnlySortedMetadata(t *testing.T) {
	store := NewInMemoryCredentialStore(map[string]Credential{
		"z-oauth": {Type: CredentialTypeOAuth, Access: "secret", Refresh: "secret"},
		"a-key":   {Type: CredentialTypeAPIKey, Key: "secret"},
	})

	got, err := store.ListCredentials(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []CredentialInfo{
		{ProviderID: "a-key", Type: CredentialTypeAPIKey},
		{ProviderID: "z-oauth", Type: CredentialTypeOAuth},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("credentials = %#v, want %#v", got, want)
	}
}

func TestInMemoryCredentialStoreSerializesEachProvider(t *testing.T) {
	store := NewInMemoryCredentialStore(map[string]Credential{
		"provider": {Type: CredentialTypeAPIKey, Key: "0"},
	})

	const workers = 32
	var wait sync.WaitGroup
	wait.Add(workers)
	errorsByWorker := make(chan error, workers)
	for range workers {
		go func() {
			defer wait.Done()
			_, _, err := store.ModifyCredential(
				context.Background(),
				"provider",
				func(_ context.Context, current Credential, _ bool) (Credential, bool, error) {
					value, err := strconv.Atoi(current.Key)
					if err != nil {
						return Credential{}, false, err
					}
					current.Key = strconv.Itoa(value + 1)
					return current, true, nil
				},
			)
			errorsByWorker <- err
		}()
	}
	wait.Wait()
	close(errorsByWorker)
	for err := range errorsByWorker {
		if err != nil {
			t.Fatal(err)
		}
	}

	credential, _, err := store.ReadCredential(context.Background(), "provider")
	if err != nil {
		t.Fatal(err)
	}
	if credential.Key != strconv.Itoa(workers) {
		t.Fatalf("serialized value = %q, want %d", credential.Key, workers)
	}
}

func TestInMemoryCredentialStoreDoesNotGloballySerializeProviders(t *testing.T) {
	store := NewInMemoryCredentialStore()
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		_, _, err := store.ModifyCredential(
			context.Background(),
			"first",
			func(_ context.Context, _ Credential, _ bool) (Credential, bool, error) {
				close(firstStarted)
				<-releaseFirst
				return Credential{Type: CredentialTypeAPIKey, Key: "first"}, true, nil
			},
		)
		firstDone <- err
	}()
	<-firstStarted

	secondDone := make(chan error, 1)
	go func() {
		_, _, err := store.ModifyCredential(
			context.Background(),
			"second",
			func(_ context.Context, _ Credential, _ bool) (Credential, bool, error) {
				return Credential{Type: CredentialTypeAPIKey, Key: "second"}, true, nil
			},
		)
		secondDone <- err
	}()

	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("second provider was blocked by first provider")
	}
	close(releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
}

func TestInMemoryCredentialStorePreservesStateOnModifierError(t *testing.T) {
	sentinel := errors.New("refresh failed")
	store := NewInMemoryCredentialStore(map[string]Credential{
		"provider": {Type: CredentialTypeAPIKey, Key: "original"},
	})

	_, _, err := store.ModifyCredential(
		context.Background(),
		"provider",
		func(_ context.Context, current Credential, _ bool) (Credential, bool, error) {
			current.Key = "discarded"
			return current, true, sentinel
		},
	)
	if !errors.Is(err, sentinel) {
		t.Fatalf("modify error = %v", err)
	}
	credential, _, err := store.ReadCredential(context.Background(), "provider")
	if err != nil {
		t.Fatal(err)
	}
	if credential.Key != "original" {
		t.Fatalf("credential changed after modifier error: %#v", credential)
	}
}

func TestInMemoryCredentialStoreCommitsSuccessfulModifierAfterCancellation(t *testing.T) {
	store := NewInMemoryCredentialStore(map[string]Credential{
		"provider": {Type: CredentialTypeOAuth, Access: "old", Refresh: "old-refresh"},
	})
	ctx, cancel := context.WithCancel(context.Background())

	written, exists, err := store.ModifyCredential(
		ctx,
		"provider",
		func(_ context.Context, current Credential, _ bool) (Credential, bool, error) {
			cancel()
			current.Access = "fresh"
			current.Refresh = "rotated"
			return current, true, nil
		},
	)
	if err != nil || !exists {
		t.Fatalf("modify result: exists=%v err=%v", exists, err)
	}
	if written.Access != "fresh" || written.Refresh != "rotated" {
		t.Fatalf("written credential = %#v", written)
	}
	persisted, exists, err := store.ReadCredential(context.Background(), "provider")
	if err != nil || !exists {
		t.Fatalf("read result: exists=%v err=%v", exists, err)
	}
	if persisted.Access != "fresh" || persisted.Refresh != "rotated" {
		t.Fatalf("persisted credential = %#v", persisted)
	}
}

func TestInMemoryCredentialStoreWaitHonorsContextCancellation(t *testing.T) {
	store := NewInMemoryCredentialStore()
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		_, _, err := store.ModifyCredential(
			context.Background(),
			"provider",
			func(_ context.Context, _ Credential, _ bool) (Credential, bool, error) {
				close(firstStarted)
				<-releaseFirst
				return Credential{Type: CredentialTypeAPIKey, Key: "first"}, true, nil
			},
		)
		firstDone <- err
	}()
	<-firstStarted

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := store.ModifyCredential(
		ctx,
		"provider",
		func(_ context.Context, _ Credential, _ bool) (Credential, bool, error) {
			t.Fatal("modifier ran after cancellation")
			return Credential{}, false, nil
		},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("wait error = %v, want context.Canceled", err)
	}

	close(releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
}

func TestInMemoryCredentialStoreDeleteSerializesWithModify(t *testing.T) {
	store := NewInMemoryCredentialStore()
	modifyStarted := make(chan struct{})
	releaseModify := make(chan struct{})
	modifyDone := make(chan error, 1)
	go func() {
		_, _, err := store.ModifyCredential(
			context.Background(),
			"provider",
			func(_ context.Context, _ Credential, _ bool) (Credential, bool, error) {
				close(modifyStarted)
				<-releaseModify
				return Credential{Type: CredentialTypeAPIKey, Key: "written"}, true, nil
			},
		)
		modifyDone <- err
	}()
	<-modifyStarted

	deleteDone := make(chan error, 1)
	go func() {
		deleteDone <- store.DeleteCredential(context.Background(), "provider")
	}()
	select {
	case err := <-deleteDone:
		t.Fatalf("delete completed before modify: %v", err)
	case <-time.After(25 * time.Millisecond):
	}

	close(releaseModify)
	if err := <-modifyDone; err != nil {
		t.Fatal(err)
	}
	if err := <-deleteDone; err != nil {
		t.Fatal(err)
	}
	if _, exists, err := store.ReadCredential(context.Background(), "provider"); err != nil || exists {
		t.Fatalf("credential after delete: exists=%v err=%v", exists, err)
	}
}
