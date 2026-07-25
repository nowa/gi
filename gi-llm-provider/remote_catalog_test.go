package gillmprovider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestWithRemoteCatalogRefreshLifecycle(t *testing.T) {
	t.Run("keyed catalog uses attribution TTL and forced refresh", func(t *testing.T) {
		var calls atomic.Int32
		var userAgent string
		client := remoteCatalogHTTPDoerFunc(func(
			request *http.Request,
		) (*http.Response, error) {
			calls.Add(1)
			userAgent = request.Header.Get("User-Agent")
			if request.URL.Path != "/api/models/providers/test-provider" {
				t.Errorf("request path = %q", request.URL.Path)
			}
			return remoteCatalogHTTPResponse(
				http.StatusOK,
				`{"dynamic":{"id":"dynamic","name":"Dynamic","api":"openai-completions","provider":"wrong","reasoning":false,"input":["text"],"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0},"contextWindow":1000,"maxTokens":100}}`,
				nil,
			), nil
		})

		provider := newRemoteCatalogTestProvider(
			t,
			client,
			time.Time{},
		)
		store := NewInMemoryModelsStore()
		refresh := RefreshModelsContext{
			Store:        remoteCatalogScopedStore{store, provider.ID},
			AllowNetwork: true,
		}
		if err := provider.RefreshModels(context.Background(), refresh); err != nil {
			t.Fatal(err)
		}
		if err := provider.RefreshModels(context.Background(), refresh); err != nil {
			t.Fatal(err)
		}
		refresh.Force = true
		if err := provider.RefreshModels(context.Background(), refresh); err != nil {
			t.Fatal(err)
		}

		if got := remoteCatalogModelIDs(t, provider); !reflect.DeepEqual(
			got,
			[]string{"static", "dynamic"},
		) {
			t.Fatalf("model IDs = %#v", got)
		}
		if calls.Load() != 2 {
			t.Fatalf("network calls = %d, want 2", calls.Load())
		}
		if userAgent != "gi/test" {
			t.Fatalf("user agent = %q", userAgent)
		}
		stored, exists, err := store.ReadModels(
			context.Background(),
			provider.ID,
		)
		if err != nil || !exists {
			t.Fatalf("stored = %#v, %v, %v", stored, exists, err)
		}
		if len(stored.Models) != 1 ||
			stored.Models[0].Provider != provider.ID ||
			stored.LastModified == nil ||
			*stored.LastModified != 0 {
			t.Fatalf("stored catalog = %#v", stored)
		}
	})

	t.Run("newer catalog wins over compiled generation time", func(t *testing.T) {
		localGeneratedAt := time.Date(
			2026,
			time.July,
			23,
			10,
			0,
			0,
			0,
			time.UTC,
		)
		var calls atomic.Int32
		client := remoteCatalogHTTPDoerFunc(func(
			_ *http.Request,
		) (*http.Response, error) {
			current := calls.Add(1)
			modified := localGeneratedAt.Add(-time.Minute)
			id := "old"
			if current > 1 {
				modified = localGeneratedAt.Add(time.Minute)
				id = "newer"
			}
			return remoteCatalogHTTPResponse(
				http.StatusOK,
				`[{"id":"`+id+`","name":"`+id+`","api":"openai-completions","reasoning":false,"input":["text"],"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0},"contextWindow":1000,"maxTokens":100}]`,
				http.Header{
					"Last-Modified": []string{
						modified.Format(http.TimeFormat),
					},
				},
			), nil
		})

		provider := newRemoteCatalogTestProvider(
			t,
			client,
			localGeneratedAt,
		)
		store := NewInMemoryModelsStore()
		refresh := RefreshModelsContext{
			Store:        remoteCatalogScopedStore{store, provider.ID},
			AllowNetwork: true,
		}
		if err := provider.RefreshModels(context.Background(), refresh); err != nil {
			t.Fatal(err)
		}
		if got := remoteCatalogModelIDs(t, provider); !reflect.DeepEqual(
			got,
			[]string{"static"},
		) {
			t.Fatalf("old remote model IDs = %#v", got)
		}
		refresh.Force = true
		if err := provider.RefreshModels(context.Background(), refresh); err != nil {
			t.Fatal(err)
		}
		if got := remoteCatalogModelIDs(t, provider); !reflect.DeepEqual(
			got,
			[]string{"static", "newer"},
		) {
			t.Fatalf("new remote model IDs = %#v", got)
		}
		stored, _, _ := store.ReadModels(context.Background(), provider.ID)
		if stored.LastModified == nil ||
			*stored.LastModified != localGeneratedAt.Add(time.Minute).UnixMilli() {
			t.Fatalf("last modified = %#v", stored.LastModified)
		}
	})

	t.Run("unimplemented endpoint records unavailable overlay", func(t *testing.T) {
		client := remoteCatalogHTTPDoerFunc(func(
			_ *http.Request,
		) (*http.Response, error) {
			return remoteCatalogHTTPResponse(
				http.StatusNotImplemented,
				"",
				nil,
			), nil
		})

		provider := newRemoteCatalogTestProvider(
			t,
			client,
			time.Time{},
		)
		store := NewInMemoryModelsStore()
		if err := provider.RefreshModels(
			context.Background(),
			RefreshModelsContext{
				Store:        remoteCatalogScopedStore{store, provider.ID},
				AllowNetwork: true,
			},
		); err != nil {
			t.Fatal(err)
		}
		if got := remoteCatalogModelIDs(t, provider); !reflect.DeepEqual(
			got,
			[]string{"static"},
		) {
			t.Fatalf("model IDs = %#v", got)
		}
		stored, exists, err := store.ReadModels(
			context.Background(),
			provider.ID,
		)
		if err != nil || !exists ||
			stored.CheckedAt == 0 ||
			stored.LastModified == nil ||
			*stored.LastModified != 0 ||
			len(stored.Models) != 0 {
			t.Fatalf("unavailable catalog = %#v, %v, %v", stored, exists, err)
		}
	})
}

func TestModelsStoreEntryPreservesLastModifiedPresence(t *testing.T) {
	zero := int64(0)
	withZero, err := json.Marshal(ModelsStoreEntry{
		Models:       []Model{},
		LastModified: &zero,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(withZero), `"lastModified":0`) {
		t.Fatalf("explicit zero encoded as %s", withZero)
	}
	withoutValue, err := json.Marshal(ModelsStoreEntry{Models: []Model{}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(withoutValue), "lastModified") {
		t.Fatalf("absent lastModified encoded as %s", withoutValue)
	}
	var decoded ModelsStoreEntry
	if err := json.Unmarshal(withZero, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.LastModified == nil || *decoded.LastModified != 0 {
		t.Fatalf("decoded lastModified = %#v", decoded.LastModified)
	}
}

func newRemoteCatalogTestProvider(
	t *testing.T,
	client HTTPDoer,
	localGeneratedAt time.Time,
) *Provider {
	t.Helper()
	baseline := []Model{remoteCatalogTestModel("static")}
	wrapped, err := WithRemoteCatalog(
		&Provider{
			ID:   "test-provider",
			Name: "Test Provider",
			ModelSource: func() ([]Model, error) {
				return cloneModels(baseline), nil
			},
		},
		RemoteCatalogOptions{
			BaseURL:          "https://catalog.test",
			Client:           client,
			UserAgent:        "gi/test",
			LocalGeneratedAt: localGeneratedAt,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return wrapped
}

func remoteCatalogTestModel(id string) Model {
	return Model{
		ID:            id,
		Name:          id,
		API:           "openai-completions",
		Provider:      "test-provider",
		BaseURL:       "https://example.test/v1",
		Input:         []string{"text"},
		ContextWindow: 1000,
		MaxTokens:     100,
	}
}

func remoteCatalogModelIDs(
	t *testing.T,
	provider *Provider,
) []string {
	t.Helper()
	models, err := provider.GetModels()
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(models))
	for _, model := range models {
		ids = append(ids, model.ID)
	}
	return ids
}

type remoteCatalogScopedStore struct {
	store      ModelsStore
	providerID string
}

func (s remoteCatalogScopedStore) ReadModels(
	ctx context.Context,
) (ModelsStoreEntry, bool, error) {
	return s.store.ReadModels(ctx, s.providerID)
}

func (s remoteCatalogScopedStore) WriteModels(
	ctx context.Context,
	entry ModelsStoreEntry,
) error {
	return s.store.WriteModels(ctx, s.providerID, entry)
}

func (s remoteCatalogScopedStore) DeleteModels(
	ctx context.Context,
) error {
	return s.store.DeleteModels(ctx, s.providerID)
}

type remoteCatalogHTTPDoerFunc func(
	*http.Request,
) (*http.Response, error)

func (f remoteCatalogHTTPDoerFunc) Do(
	request *http.Request,
) (*http.Response, error) {
	return f(request)
}

func remoteCatalogHTTPResponse(
	status int,
	body string,
	headers http.Header,
) *http.Response {
	if headers == nil {
		headers = http.Header{}
	}
	return &http.Response{
		StatusCode: status,
		Header:     headers,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
