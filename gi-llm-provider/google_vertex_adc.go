package gillmprovider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	maxGoogleCredentialsFileBytes = 10 << 20
	googleVertexTokenRefreshSkew  = time.Minute
)

type googleVertexTokenLoader func(
	context.Context,
	GoogleVertexTokenRequest,
) (*oauth2.Token, error)

type googleVertexTokenCacheEntry struct {
	token   *oauth2.Token
	refresh *googleVertexTokenRefresh
}

type googleVertexTokenRefresh struct {
	done chan struct{}
	err  error
}

// defaultGoogleVertexTokenProvider caches token values rather than token
// sources. Google token sources retain the context used to create them, so
// keeping only the resulting value prevents one canceled request from
// poisoning later requests.
type defaultGoogleVertexTokenProvider struct {
	mu          sync.Mutex
	entries     map[string]*googleVertexTokenCacheEntry
	load        googleVertexTokenLoader
	now         func() time.Time
	refreshSkew time.Duration
}

func newDefaultGoogleVertexTokenProvider() *defaultGoogleVertexTokenProvider {
	return &defaultGoogleVertexTokenProvider{
		entries:     make(map[string]*googleVertexTokenCacheEntry),
		load:        loadGoogleVertexToken,
		now:         time.Now,
		refreshSkew: googleVertexTokenRefreshSkew,
	}
}

func (p *defaultGoogleVertexTokenProvider) AccessToken(
	ctx context.Context,
	request GoogleVertexTokenRequest,
) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if p == nil {
		return "", errors.New("Google Vertex token provider is nil")
	}
	p.initialize()
	key := googleVertexTokenCacheKey(request)

	for {
		p.mu.Lock()
		entry := p.entries[key]
		if entry == nil {
			entry = &googleVertexTokenCacheEntry{}
			p.entries[key] = entry
		}
		if googleVertexTokenValid(entry.token, p.now(), p.refreshSkew) {
			accessToken := entry.token.AccessToken
			p.mu.Unlock()
			return accessToken, nil
		}
		if entry.refresh != nil {
			refresh := entry.refresh
			p.mu.Unlock()
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-refresh.done:
				if refresh.err != nil {
					return "", googleVertexTokenError(refresh.err)
				}
				continue
			}
		}
		refresh := &googleVertexTokenRefresh{done: make(chan struct{})}
		entry.refresh = refresh
		p.mu.Unlock()

		token, err := p.load(ctx, request)
		if err == nil && !googleVertexTokenUsable(token) {
			err = errors.New("Google application default credentials returned an empty access token")
		}

		p.mu.Lock()
		if err == nil {
			entry.token = token
		}
		entry.refresh = nil
		refresh.err = err
		close(refresh.done)
		p.mu.Unlock()
		if err != nil {
			return "", googleVertexTokenError(err)
		}
		return token.AccessToken, nil
	}
}

func googleVertexTokenError(err error) error {
	return fmt.Errorf("resolve Google application default credentials: %w", err)
}

func (p *defaultGoogleVertexTokenProvider) initialize() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.entries == nil {
		p.entries = make(map[string]*googleVertexTokenCacheEntry)
	}
	if p.load == nil {
		p.load = loadGoogleVertexToken
	}
	if p.now == nil {
		p.now = time.Now
	}
	if p.refreshSkew == 0 {
		p.refreshSkew = googleVertexTokenRefreshSkew
	}
}

func googleVertexTokenCacheKey(request GoogleVertexTokenRequest) string {
	keyFilename := ""
	if request.AuthOptions != nil {
		keyFilename = strings.TrimSpace(request.AuthOptions.KeyFilename)
	}
	if keyFilename == "" {
		keyFilename = "ambient"
	}
	return keyFilename
}

func googleVertexTokenUsable(token *oauth2.Token) bool {
	return token != nil && strings.TrimSpace(token.AccessToken) != ""
}

func googleVertexTokenValid(token *oauth2.Token, now time.Time, refreshSkew time.Duration) bool {
	if !googleVertexTokenUsable(token) {
		return false
	}
	return token.Expiry.IsZero() || now.Add(refreshSkew).Before(token.Expiry)
}

func loadGoogleVertexToken(
	ctx context.Context,
	request GoogleVertexTokenRequest,
) (*oauth2.Token, error) {
	params := google.CredentialsParams{Scopes: []string{GoogleCloudPlatformScope}}
	var (
		credentials *google.Credentials
		err         error
	)
	if request.AuthOptions != nil &&
		strings.TrimSpace(request.AuthOptions.KeyFilename) != "" {
		filename := strings.TrimSpace(request.AuthOptions.KeyFilename)
		contents, readErr := readGoogleCredentialsFile(filename)
		if readErr != nil {
			return nil, readErr
		}
		credentials, err = google.CredentialsFromJSONWithParams(ctx, contents, params)
		if err != nil {
			return nil, fmt.Errorf("parse Google credentials file %q: %w", filename, err)
		}
	} else {
		credentials, err = google.FindDefaultCredentialsWithParams(ctx, params)
		if err != nil {
			return nil, err
		}
	}
	token, err := credentials.TokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("retrieve Google access token: %w", err)
	}
	return token, nil
}

func readGoogleCredentialsFile(filename string) ([]byte, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("open Google credentials file %q: %w", filename, err)
	}
	defer file.Close()

	contents, err := io.ReadAll(io.LimitReader(file, maxGoogleCredentialsFileBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read Google credentials file %q: %w", filename, err)
	}
	if len(contents) > maxGoogleCredentialsFileBytes {
		return nil, fmt.Errorf(
			"Google credentials file %q exceeds %d bytes",
			filename,
			maxGoogleCredentialsFileBytes,
		)
	}
	return contents, nil
}

var _ GoogleVertexTokenProvider = (*defaultGoogleVertexTokenProvider)(nil)
