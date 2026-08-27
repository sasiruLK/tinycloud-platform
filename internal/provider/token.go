package provider

import (
	"fmt"
	"os"
	"strings"
)

// TokenSource yields the bearer token shared between Core and one Provider.
//
// It is a function rather than a string so that rotation is routine: a token
// read from a mounted Secret is re-read on every call, so replacing the Secret
// rotates the credential without redeploying either side. Kubernetes updates a
// mounted Secret's contents in place within a minute or two of the change.
type TokenSource func() (string, error)

// StaticToken returns a source yielding a fixed token. Convenient for tests and
// local development; in a cluster prefer FileToken, which can be rotated.
func StaticToken(token string) TokenSource {
	return func() (string, error) { return token, nil }
}

// FileToken returns a source reading path on every call, so that rotating the
// Secret behind it takes effect without a restart.
func FileToken(path string) TokenSource {
	return func() (string, error) {
		raw, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read provider token from %s: %w", path, err)
		}
		token := strings.TrimSpace(string(raw))
		if token == "" {
			return "", fmt.Errorf("provider token file %s is empty", path)
		}
		return token, nil
	}
}
