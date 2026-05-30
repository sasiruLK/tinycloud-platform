package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/sasiruLK/tinycloud-platform/internal/api/response"
)

// GitHubRepo represents a repository for the autocomplete picker
type GitHubRepo struct {
	Name          string `json:"name"`
	FullName      string `json:"fullName"`
	URL           string `json:"url"`
	DefaultBranch string `json:"defaultBranch"`
	Language      string `json:"language"`
	Private       bool   `json:"private"`
}

type githubRepoRaw struct {
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	HTMLURL       string `json:"html_url"`
	DefaultBranch string `json:"default_branch"`
	Language      string `json:"language"`
	Private       bool   `json:"private"`
}

var githubRepoCache struct {
	repos   []GitHubRepo
	fetched time.Time
}

const githubRepoCacheDuration = 60 * time.Second

// ListGitHubRepos returns the authenticated user's GitHub repositories
func (h *Handler) ListGitHubRepos(c *fiber.Ctx) error {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return response.JSONError(c, fiber.StatusServiceUnavailable, "github_not_configured",
			"GitHub token is not configured on the server")
	}

	// Simple in-memory cache to avoid rate limits
	if len(githubRepoCache.repos) > 0 && time.Since(githubRepoCache.fetched) < githubRepoCacheDuration {
		return response.JSON(c, fiber.Map{"repos": githubRepoCache.repos})
	}

	repos, err := fetchGitHubRepos(token)
	if err != nil {
		return response.JSONError(c, fiber.StatusBadGateway, "github_api_error",
			fmt.Sprintf("Failed to fetch GitHub repos: %v", err))
	}

	githubRepoCache.repos = repos
	githubRepoCache.fetched = time.Now()

	return response.JSON(c, fiber.Map{"repos": repos})
}

func fetchGitHubRepos(token string) ([]GitHubRepo, error) {
	req, err := http.NewRequest(http.MethodGet, "https://api.github.com/user/repos?per_page=100&sort=pushed", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{Timeout: 15 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d", res.StatusCode)
	}

	var raw []githubRepoRaw
	if err := json.NewDecoder(res.Body).Decode(&raw); err != nil {
		return nil, err
	}

	repos := make([]GitHubRepo, 0, len(raw))
	for _, r := range raw {
		repos = append(repos, GitHubRepo{
			Name:          r.Name,
			FullName:      r.FullName,
			URL:           r.HTMLURL,
			DefaultBranch: r.DefaultBranch,
			Language:      r.Language,
			Private:       r.Private,
		})
	}
	return repos, nil
}
