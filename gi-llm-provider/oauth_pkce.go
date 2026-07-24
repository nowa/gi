package gillmprovider

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"io"
)

const oauthRandomTokenBytes = 32

// PKCE contains an RFC 7636 verifier and its S256 challenge.
type PKCE struct {
	Verifier  string
	Challenge string
}

// GeneratePKCE creates a cryptographically random PKCE verifier and challenge.
func GeneratePKCE() (PKCE, error) {
	return generatePKCE(rand.Reader)
}

func generatePKCE(random io.Reader) (PKCE, error) {
	verifier, err := randomOAuthToken(random, oauthRandomTokenBytes)
	if err != nil {
		return PKCE{}, err
	}
	sum := sha256.Sum256([]byte(verifier))
	return PKCE{
		Verifier:  verifier,
		Challenge: base64.RawURLEncoding.EncodeToString(sum[:]),
	}, nil
}

func randomOAuthToken(random io.Reader, size int) (string, error) {
	value := make([]byte, size)
	if _, err := io.ReadFull(random, value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}
