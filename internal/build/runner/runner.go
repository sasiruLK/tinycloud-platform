package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/sasiruLK/tinycloud-platform/internal/build/types"
)

type Runner struct {
	coordinatorURL string
	token          string
	workDir        string
	imagePrefix    string
	buildPlatform  string
	cacheRef       string
	githubToken    string
	pollInterval   time.Duration
	http           *http.Client
}

type Config struct {
	CoordinatorURL string
	Token          string
	WorkDir        string
	// ImagePrefix is the registry path without tag, e.g. iad.ocir.io/idzghas4xwzv/tinycloud
	ImagePrefix string
	// Registry + Owner are fallback fields used when ImagePrefix is empty.
	Registry string
	Owner    string
	// BuildPlatform sets buildx --platform (default linux/arm64). Empty on native ARM64 runner.
	BuildPlatform string
	CacheRef      string
	GitHubToken   string
	PollInterval  time.Duration
}

func New(cfg Config) *Runner {
	if cfg.WorkDir == "" {
		cfg.WorkDir = "/tmp/tinycloud-builds"
	}
	if cfg.PollInterval == 0 {
		cfg.PollInterval = 5 * time.Second
	}
	return &Runner{
		coordinatorURL: strings.TrimRight(cfg.CoordinatorURL, "/"),
		token:          cfg.Token,
		workDir:        cfg.WorkDir,
		imagePrefix:    resolveImagePrefix(cfg),
		buildPlatform:  resolveBuildPlatform(cfg.BuildPlatform),
		cacheRef:       strings.TrimSpace(cfg.CacheRef),
		githubToken:    cfg.GitHubToken,
		pollInterval:   cfg.PollInterval,
		http:           &http.Client{Timeout: 30 * time.Second},
	}
}

func resolveImagePrefix(cfg Config) string {
	if p := strings.Trim(cfg.ImagePrefix, "/"); p != "" {
		return p
	}
	registry := cfg.Registry
	if registry == "" {
		registry = "iad.ocir.io"
	}
	owner := strings.Trim(cfg.Owner, "/")
	if owner == "" {
		owner = "idzghas4xwzv/tinycloud"
	}
	return registry + "/" + owner
}

// resolveBuildPlatform returns the buildx --platform value, or "" for native builds.
func resolveBuildPlatform(explicit string) string {
	if explicit == "native" || explicit == "none" {
		return ""
	}
	if explicit != "" {
		return explicit
	}
	if runtime.GOARCH == "arm64" {
		return ""
	}
	return "linux/arm64"
}

func (r *Runner) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.pollInterval)
	defer ticker.Stop()
	for {
		if err := r.pollOnce(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "runner poll failed: %v\n", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (r *Runner) pollOnce(ctx context.Context) error {
	var res types.RunnerPollResponse
	if err := r.do(ctx, http.MethodPost, "/v1/runner/poll", nil, &res); err != nil {
		return err
	}
	if res.Job == nil {
		return nil
	}
	return r.runJob(ctx, res.Job)
}

func (r *Runner) runJob(ctx context.Context, job *types.BuildJob) error {
	jobDir := filepath.Join(r.workDir, job.ID)
	_ = os.RemoveAll(jobDir)
	if err := os.MkdirAll(jobDir, 0755); err != nil {
		return err
	}
	defer os.RemoveAll(jobDir)

	r.log(ctx, job.ID, "stdout", "cloning repository")
	cloneURL := r.cloneURL(job.RepoURL)
	if err := r.run(ctx, job.ID, "", "git", "clone", "--depth", "1", "--branch", job.Ref, cloneURL, jobDir); err != nil {
		r.fail(ctx, job.ID, "clone failed: "+err.Error())
		return nil
	}

	commit, err := r.output(ctx, job.ID, jobDir, "git", "rev-parse", "HEAD")
	if err != nil {
		r.fail(ctx, job.ID, "failed to resolve commit: "+err.Error())
		return nil
	}
	commit = strings.TrimSpace(commit)

	framework, err := DetectFramework(jobDir)
	if err != nil {
		r.fail(ctx, job.ID, err.Error())
		return nil
	}
	_ = r.status(ctx, job.ID, types.RunnerStatusRequest{Status: types.StatusRunning, CommitSHA: commit, Framework: framework})

	dockerfile := filepath.Join(jobDir, "Dockerfile")
	if _, err := os.Stat(dockerfile); os.IsNotExist(err) {
		dfFramework := framework
		if framework == "node" {
			dfFramework = NodeFramework(jobDir)
		}
		r.log(ctx, job.ID, "stdout", "generating Dockerfile for "+dfFramework)
		if err := os.WriteFile(dockerfile, []byte(GeneratedDockerfile(dfFramework, job.Port, NodeStaticOutputDir(jobDir))), 0644); err != nil {
			r.fail(ctx, job.ID, "failed to write Dockerfile: "+err.Error())
			return nil
		}
	}

	image := fmt.Sprintf("%s/%s", r.imagePrefix, job.AppName)
	tag := commit
	fullImage := image + ":" + tag
	r.log(ctx, job.ID, "stdout", "building "+fullImage)
	buildArgs := r.buildArgs(fullImage)
	if err := r.run(ctx, job.ID, jobDir, buildArgs[0], buildArgs[1:]...); err != nil {
		r.fail(ctx, job.ID, "build failed: "+err.Error())
		return nil
	}
	if err := r.smokeTest(ctx, job.ID, fullImage, job.Port); err != nil {
		r.fail(ctx, job.ID, "smoke test failed: "+err.Error())
		return nil
	}
	r.log(ctx, job.ID, "stdout", "smoke test passed")
	if err := r.run(ctx, job.ID, jobDir, "docker", "push", fullImage); err != nil {
		r.fail(ctx, job.ID, "push failed: "+err.Error())
		return nil
	}
	r.log(ctx, job.ID, "stdout", "pushed "+fullImage)
	return r.status(ctx, job.ID, types.RunnerStatusRequest{
		Status: types.StatusSucceeded, CommitSHA: commit, Framework: framework, Image: image, Tag: tag,
	})
}

func (r *Runner) buildArgs(fullImage string) []string {
	args := []string{"docker", "buildx", "build", "-t", fullImage}
	if r.buildPlatform != "" {
		args = append(args, "--platform", r.buildPlatform)
	}
	if r.cacheRef != "" {
		args = append(args,
			"--cache-from", "type=registry,ref="+r.cacheRef,
			"--cache-to", "type=registry,ref="+r.cacheRef+",mode=max",
		)
	}
	args = append(args, "--load", ".")
	return args
}

func (r *Runner) cloneURL(repoURL string) string {
	if r.githubToken == "" || !strings.HasPrefix(repoURL, "https://github.com/") {
		return repoURL
	}
	return strings.Replace(repoURL, "https://github.com/", "https://x-access-token:"+r.githubToken+"@github.com/", 1)
}

func (r *Runner) redact(message string) string {
	if r.githubToken == "" {
		return message
	}
	return strings.ReplaceAll(message, r.githubToken, "REDACTED")
}

func DetectFramework(dir string) (string, error) {
	if _, err := os.Stat(filepath.Join(dir, "package.json")); err == nil {
		return "node", nil
	}
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
		return "go", nil
	}
	return "", fmt.Errorf("unsupported framework: expected package.json or go.mod")
}

// NodeFramework returns "node-server" for apps that need a runtime Node process
// (Next.js, Express, etc.) or "node-static" for apps that compile to static files.
func NodeFramework(workDir string) string {
	data, err := os.ReadFile(filepath.Join(workDir, "package.json"))
	if err != nil {
		return "node-static"
	}
	s := string(data)
	if strings.Contains(s, `"next"`) {
		return "node-server"
	}
	return "node-static"
}

func NodeStaticOutputDir(workDir string) string {
	data, err := os.ReadFile(filepath.Join(workDir, "package.json"))
	if err != nil {
		return "dist"
	}
	if strings.Contains(string(data), "react-scripts") {
		return "build"
	}
	return "dist"
}

func GeneratedDockerfile(framework string, port int, nodeStaticRoot string) string {
	switch framework {
	case "node-server":
		return fmt.Sprintf(`FROM node:22-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
ENV NODE_ENV=production
RUN npm run build

FROM node:22-alpine
WORKDIR /app
ENV NODE_ENV=production
ENV PORT=%d
COPY --from=builder /app/package.json ./
COPY --from=builder /app/.next ./.next
COPY --from=builder /app/node_modules ./node_modules
COPY --from=builder /app/public ./public
EXPOSE %d
CMD ["npm", "start"]
`, port, port)
	case "node", "node-static":
		if nodeStaticRoot == "" {
			nodeStaticRoot = "dist"
		}
		return fmt.Sprintf(`FROM node:22-alpine AS build
WORKDIR /app
COPY package*.json ./
RUN npm install
COPY . .
RUN npm run build

FROM nginx:alpine
COPY --from=build /app/%s /usr/share/nginx/html
RUN sed -i 's/listen       80;/listen       %d;/' /etc/nginx/conf.d/default.conf
EXPOSE %d
`, nodeStaticRoot, port, port)
	case "go":
		return fmt.Sprintf(`FROM golang:1.25-alpine AS build
WORKDIR /src
COPY . .
RUN go mod download
RUN find . -name "*.go" -exec sed -i \
	-e 's/localhost:[0-9]\+/0.0.0.0:%d/g' \
	-e 's/"127\.0\.0\.1:[0-9]\+"/"0.0.0.0:%d"/g' \
	-e 's/":8080"/":%d"/g' {} + 2>/dev/null || true
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-w -s" -o /server .

FROM gcr.io/distroless/static:nonroot
WORKDIR /app
COPY --from=build /server /app/server
USER 65532:65532
ENV PORT=%d
EXPOSE %d
ENTRYPOINT ["/app/server"]
`, port, port, port, port, port)
	default:
		return ""
	}
}

func (r *Runner) smokeTest(ctx context.Context, jobID, fullImage string, port int) error {
	if port == 0 {
		port = 8080
	}
	containerName := "tinycloud-smoke-" + jobID
	defer func() {
		_ = r.run(ctx, jobID, "", "docker", "rm", "-f", containerName)
	}()

	if err := r.run(ctx, jobID, "", "docker", "run", "-d", "--name", containerName,
		"-e", fmt.Sprintf("PORT=%d", port),
		"-p", fmt.Sprintf("127.0.0.1:%d:%d", port, port),
		fullImage); err != nil {
		return fmt.Errorf("container failed to start: %w", err)
	}

	url := fmt.Sprintf("http://127.0.0.1:%d/", port)
	var lastErr error
	for attempt := 0; attempt < 10; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
		cmd := exec.CommandContext(ctx, "curl", "-fsS", "-o", "/dev/null", url)
		if err := cmd.Run(); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}
	return fmt.Errorf("service did not respond on %s: %w", url, lastErr)
}

func (r *Runner) run(ctx context.Context, jobID, dir, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if out.Len() > 0 {
		r.log(ctx, jobID, "stdout", r.redact(strings.TrimRight(out.String(), "\n")))
	}
	return err
}

func (r *Runner) output(ctx context.Context, jobID, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if len(out) > 0 {
		r.log(ctx, jobID, "stdout", r.redact(strings.TrimRight(string(out), "\n")))
	}
	return string(out), err
}

func (r *Runner) fail(ctx context.Context, jobID, message string) {
	r.log(ctx, jobID, "stderr", message)
	_ = r.status(ctx, jobID, types.RunnerStatusRequest{Status: types.StatusFailed, Error: message})
}

func (r *Runner) log(ctx context.Context, jobID, stream, message string) {
	_ = r.do(ctx, http.MethodPost, "/v1/runner/jobs/"+jobID+"/logs", types.RunnerLogRequest{Stream: stream, Message: message}, nil)
}

func (r *Runner) status(ctx context.Context, jobID string, req types.RunnerStatusRequest) error {
	return r.do(ctx, http.MethodPost, "/v1/runner/jobs/"+jobID+"/status", req, nil)
}

func (r *Runner) do(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return err
		}
		reader = &buf
	}
	req, err := http.NewRequestWithContext(ctx, method, r.coordinatorURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if r.token != "" {
		req.Header.Set("Authorization", "Bearer "+r.token)
	}
	res, err := r.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		b, _ := io.ReadAll(res.Body)
		return fmt.Errorf("%s: %s", res.Status, strings.TrimSpace(string(b)))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(res.Body).Decode(out)
}
