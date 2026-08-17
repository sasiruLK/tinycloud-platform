package coordinator

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func sign(secret string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

// This is the only thing standing between a public endpoint and the ability to
// trigger builds, so it gets the unhappy paths rather than just the happy one.
func TestValidSignature(t *testing.T) {
	secret := "s3cr3t"
	body := []byte(`{"ref":"refs/heads/main"}`)
	good := sign(secret, body)

	cases := []struct {
		name   string
		secret string
		body   []byte
		header string
		want   bool
	}{
		{"correct signature", secret, body, good, true},
		{"wrong secret", "other", body, good, false},
		{"body tampered after signing", secret, []byte(`{"ref":"refs/heads/evil"}`), good, false},
		{"empty header", secret, body, "", false},
		{"missing sha256 prefix", secret, body, hex.EncodeToString([]byte("x")), false},
		{"sha1 style header", secret, body, "sha1=" + hex.EncodeToString([]byte("x")), false},
		{"not hex", secret, body, "sha256=zzzz", false},
		{"truncated but matching prefix", secret, body, good[:20], false},
		{"empty secret rejects anything", "", body, good, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validSignature(tc.secret, tc.body, tc.header); got != tc.want {
				t.Fatalf("validSignature = %v, want %v", got, tc.want)
			}
		})
	}
}

// A push arrives as clone_url (with .git); apps are stored from whatever the
// user typed. If these do not compare equal, a push silently rebuilds nothing.
func TestNormalizeRepoMatchesTheFormsGitHubSends(t *testing.T) {
	want := "github.com/sasirulk/htmxgo-counter"
	for _, in := range []string{
		"https://github.com/sasiruLK/htmxgo-counter",
		"https://github.com/sasiruLK/htmxgo-counter.git",
		"https://github.com/sasiruLK/htmxgo-counter/",
		"http://github.com/sasiruLK/htmxgo-counter",
		"git@github.com:sasiruLK/htmxgo-counter.git",
		"HTTPS://GitHub.com/SasiruLK/HTMXGo-Counter",
	} {
		if got := normalizeRepo(in); got != want {
			t.Errorf("normalizeRepo(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeRepoKeepsDifferentReposDistinct(t *testing.T) {
	a := normalizeRepo("https://github.com/sasiruLK/blog")
	b := normalizeRepo("https://github.com/sasiruLK/blog2")
	if a == b {
		t.Fatal("blog and blog2 must not normalize to the same repo")
	}
}
