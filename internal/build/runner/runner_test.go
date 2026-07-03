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

func TestNodeStaticOutputDir(t *testing.T) {
	dir := t.TempDir()
	require.Equal(t, "dist", NodeStaticOutputDir(dir))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"dependencies":{"react-scripts":"5.0.1"}}`), 0644))
	require.Equal(t, "build", NodeStaticOutputDir(dir))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"devDependencies":{"vite":"5.0.0"}}`), 0644))
	require.Equal(t, "dist", NodeStaticOutputDir(dir))
}

func TestGeneratedDockerfile(t *testing.T) {
	node := GeneratedDockerfile("node", 3000, "dist")
	require.Contains(t, node, "FROM node:22-alpine")
	require.Contains(t, node, "/app/dist /usr/share/nginx/html")
	require.Contains(t, node, "listen       3000")
	require.Contains(t, node, "EXPOSE 3000")

	cra := GeneratedDockerfile("node", 8080, "build")
	require.Contains(t, cra, "/app/build /usr/share/nginx/html")

	goFile := GeneratedDockerfile("go", 8080, "")
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

func TestResolveImagePrefix(t *testing.T) {
	require.Equal(t, "iad.ocir.io/ns/tinycloud", resolveImagePrefix(Config{
		ImagePrefix: "iad.ocir.io/ns/tinycloud",
	}))
	require.Equal(t, "iad.ocir.io/idzghas4xwzv/tinycloud", resolveImagePrefix(Config{}))
	require.Equal(t, "iad.ocir.io/customns/tinycloud", resolveImagePrefix(Config{
		Registry: "iad.ocir.io",
		Owner:    "customns/tinycloud",
	}))
}

func TestBuildArgsNativeARM64WithCache(t *testing.T) {
	r := New(Config{
		ImagePrefix:   "iad.ocir.io/ns/tinycloud",
		BuildPlatform: "native",
		CacheRef:      "iad.ocir.io/ns/tinycloud/cache:buildkit",
	})
	args := r.buildArgs("iad.ocir.io/ns/tinycloud/app:abc123")
	require.Equal(t, []string{
		"docker", "buildx", "build", "-t", "iad.ocir.io/ns/tinycloud/app:abc123",
		"--cache-from", "type=registry,ref=iad.ocir.io/ns/tinycloud/cache:buildkit",
		"--cache-to", "type=registry,ref=iad.ocir.io/ns/tinycloud/cache:buildkit,mode=max",
		"--load", ".",
	}, args)
}

func TestBuildArgsCrossCompile(t *testing.T) {
	r := New(Config{
		ImagePrefix:   "iad.ocir.io/user/tinycloud",
		BuildPlatform: "linux/arm64",
	})
	args := r.buildArgs("iad.ocir.io/user/tinycloud/app:tag")
	require.Contains(t, args, "--platform")
	require.Contains(t, args, "linux/arm64")
}
