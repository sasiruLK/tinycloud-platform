package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDetectFramework(t *testing.T) {
	dir := t.TempDir()

	_, err := DetectFramework(dir)
	require.Error(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"scripts":{"build":"vite build"}}`), 0644))
	framework, err := DetectFramework(dir)
	require.NoError(t, err)
	require.Equal(t, "node", framework)
}

func TestGeneratedDockerfile(t *testing.T) {
	node := GeneratedDockerfile("node", 3000)
	require.Contains(t, node, "FROM node:22-alpine")
	require.Contains(t, node, "listen       3000")
	require.Contains(t, node, "EXPOSE 3000")

	goFile := GeneratedDockerfile("go", 8080)
	require.Contains(t, goFile, "GOARCH=arm64")
	require.Contains(t, goFile, "EXPOSE 8080")
	require.Contains(t, goFile, `ENTRYPOINT ["/app/server"]`)
	require.Contains(t, goFile, "COPY --from=build /server /app/server")
	require.NotContains(t, goFile, `ENTRYPOINT ["/app"]`)
}

func TestCloneURLRedactsGitHubToken(t *testing.T) {
	r := New(Config{GitHubToken: "secret-token"})
	url := r.cloneURL("https://github.com/user/repo")
	require.True(t, strings.Contains(url, "secret-token"))
	require.NotContains(t, r.redact(url), "secret-token")
}
