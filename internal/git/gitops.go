package git

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"gopkg.in/yaml.v2"
)

const (
	GitOpsRepoURL = "https://github.com/sasiruLK/gitops-lab.git"
	RepoName      = "gitops-lab"
)

// RollbackEntry represents a single rollback/restore event in rollbacks.yaml
type RollbackEntry struct {
	ID                 string `yaml:"id"`
	Type               string `yaml:"type"`
	Timestamp          string `yaml:"timestamp"`
	TargetRevision     string `yaml:"targetRevision,omitempty"`
	TargetImage        string `yaml:"targetImage,omitempty"`
	PreviousRevision   string `yaml:"previousRevision,omitempty"`
	PreviousImage      string `yaml:"previousImage,omitempty"`
	RestoredToRevision string `yaml:"restoredToRevision,omitempty"`
	RestoredToImage    string `yaml:"restoredToImage,omitempty"`
	Reason             string `yaml:"reason"`
	RollbackBranch     string `yaml:"rollbackBranch,omitempty"`
	InitiatedBy        string `yaml:"initiatedBy"`
}

// RollbacksFile is the structure of rollbacks/rollbacks.yaml
type RollbacksFile struct {
	Version   string                    `yaml:"version"`
	Schema    string                    `yaml:"schema"`
	GeneratedAt string                  `yaml:"generatedAt"`
	Apps      map[string]*AppRollbacks `yaml:"apps"`
}

// AppRollbacks holds rollback state for a single app
type AppRollbacks struct {
	CurrentStatus  string           `yaml:"currentStatus"`
	ActiveRollback *RollbackEntry   `yaml:"activeRollback"`
	History        []*RollbackEntry `yaml:"history"`
}

// GitOps handles git operations on the GitOps repo
type GitOps struct {
	Username string
	Token    string
	WorkDir  string
}

// NewGitOps creates a GitOps client from env vars
func NewGitOps() *GitOps {
	return &GitOps{
		Username: os.Getenv("GITHUB_USERNAME"),
		Token:    os.Getenv("GITHUB_TOKEN"),
		WorkDir:  "/tmp/gitops-lab",
	}
}

// Clone clones the GitOps repo
func (g *GitOps) Clone() (*git.Repository, error) {
	// Remove existing workdir
	os.RemoveAll(g.WorkDir)

	url := GitOpsRepoURL
	if g.Username != "" && g.Token != "" {
		url = fmt.Sprintf("https://%s:%s@github.com/sasiruLK/gitops-lab.git", g.Username, g.Token)
	}

	repo, err := git.PlainClone(g.WorkDir, false, &git.CloneOptions{
		URL:      url,
		Progress: os.Stderr,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to clone repo: %w", err)
	}

	return repo, nil
}

// PathExists checks if a path exists in the cloned repo on main
func (g *GitOps) PathExists(relPath string) (bool, error) {
	repo, err := g.Clone()
	if err != nil {
		return false, err
	}

	w, err := repo.Worktree()
	if err != nil {
		return false, err
	}

	if err := w.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("main"),
	}); err != nil {
		return false, fmt.Errorf("failed to checkout main: %w", err)
	}

	fullPath := filepath.Join(g.WorkDir, relPath)
	_, err = os.Stat(fullPath)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// CommitFiles writes multiple files, commits, and pushes to main
func (g *GitOps) CommitFiles(message string, files map[string][]byte, author string) error {
	repo, err := g.Clone()
	if err != nil {
		return err
	}

	w, err := repo.Worktree()
	if err != nil {
		return err
	}

	if err := w.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("main"),
	}); err != nil {
		return fmt.Errorf("failed to checkout main: %w", err)
	}

	for relPath, content := range files {
		fullPath := filepath.Join(g.WorkDir, relPath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return fmt.Errorf("failed to create directory for %s: %w", relPath, err)
		}
		if err := os.WriteFile(fullPath, content, 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", relPath, err)
		}
		if _, err := w.Add(relPath); err != nil {
			return fmt.Errorf("failed to stage %s: %w", relPath, err)
		}
	}

	if author == "" {
		author = "tinycloud-api"
	}

	_, err = w.Commit(message, &git.CommitOptions{
		Author: &object.Signature{
			Name:  author,
			Email: fmt.Sprintf("%s@tinycloud.local", author),
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	err = repo.Push(&git.PushOptions{
		Auth: &http.BasicAuth{
			Username: g.Username,
			Password: g.Token,
		},
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("failed to push commit: %w", err)
	}

	return nil
}

// UpdateDeploymentReplicas sets replicas in apps/{app}/deployment.yaml and commits
func (g *GitOps) UpdateDeploymentReplicas(app string, replicas int, author string) error {
	repo, err := g.Clone()
	if err != nil {
		return err
	}

	w, err := repo.Worktree()
	if err != nil {
		return err
	}

	if err := w.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("main"),
	}); err != nil {
		return fmt.Errorf("failed to checkout main: %w", err)
	}

	relPath := filepath.Join("apps", app, "deployment.yaml")
	fullPath := filepath.Join(g.WorkDir, relPath)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return fmt.Errorf("deployment manifest not found for app %s: %w", app, err)
	}

	updated, err := patchDeploymentReplicas(data, replicas)
	if err != nil {
		return err
	}

	if err := os.WriteFile(fullPath, updated, 0644); err != nil {
		return fmt.Errorf("failed to write deployment: %w", err)
	}

	if _, err := w.Add(filepath.ToSlash(filepath.Join("apps", app, "deployment.yaml"))); err != nil {
		return fmt.Errorf("failed to stage deployment: %w", err)
	}

	if author == "" {
		author = "tinycloud-api"
	}

	msg := fmt.Sprintf("suspend(%s): scale to %d replicas", app, replicas)
	_, err = w.Commit(msg, &git.CommitOptions{
		Author: &object.Signature{
			Name:  author,
			Email: fmt.Sprintf("%s@tinycloud.local", author),
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	err = repo.Push(&git.PushOptions{
		Auth: &http.BasicAuth{
			Username: g.Username,
			Password: g.Token,
		},
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("failed to push commit: %w", err)
	}

	return nil
}

func patchDeploymentReplicas(data []byte, replicas int) ([]byte, error) {
	var doc map[string]interface{}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("failed to parse deployment yaml: %w", err)
	}

	spec, ok := doc["spec"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("deployment spec not found")
	}
	spec["replicas"] = replicas

	out, err := yaml.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal deployment yaml: %w", err)
	}
	return out, nil
}

// CreateRollbackBranch creates a rollback branch at the target commit
func (g *GitOps) CreateRollbackBranch(app, targetSHA string) error {
	repo, err := g.Clone()
	if err != nil {
		return err
	}

	branchName := fmt.Sprintf("rollback/%s", app)

	// Create branch at target SHA
	hash := plumbing.NewHash(targetSHA)
	refName := plumbing.NewBranchReferenceName(branchName)

	err = repo.CreateBranch(&config.Branch{
		Name:   branchName,
		Remote: "origin",
		Merge:  refName,
	})
	if err != nil {
		return fmt.Errorf("failed to create branch config: %w", err)
	}

	// Set branch reference to target hash
	newRef := plumbing.NewHashReference(refName, hash)
	err = repo.Storer.SetReference(newRef)
	if err != nil {
		return fmt.Errorf("failed to set branch ref: %w", err)
	}

	// Push branch with force
	err = repo.Push(&git.PushOptions{
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{config.RefSpec(fmt.Sprintf("+%s:%s", refName, refName))},
		Auth: &http.BasicAuth{
			Username: g.Username,
			Password: g.Token,
		},
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("failed to push rollback branch: %w", err)
	}

	return nil
}

// ReadRollbacks reads rollbacks/rollbacks.yaml from the repo
func (g *GitOps) ReadRollbacks() (*RollbacksFile, error) {
	repo, err := g.Clone()
	if err != nil {
		return nil, err
	}

	// Checkout main to read latest rollbacks.yaml
	w, err := repo.Worktree()
	if err != nil {
		return nil, err
	}

	err = w.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("main"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to checkout main: %w", err)
	}

	return g.readRollbacksFromDisk()
}

func (g *GitOps) readRollbacksFromDisk() (*RollbacksFile, error) {
	path := filepath.Join(g.WorkDir, "rollbacks", "rollbacks.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Return empty structure
			return &RollbacksFile{
				Version:     "1.0",
				Schema:      "rollback-history",
				GeneratedAt: time.Now().UTC().Format(time.RFC3339),
				Apps:        make(map[string]*AppRollbacks),
			}, nil
		}
		return nil, fmt.Errorf("failed to read rollbacks file: %w", err)
	}

	var rollbacks RollbacksFile
	if err := yaml.Unmarshal(data, &rollbacks); err != nil {
		return nil, fmt.Errorf("failed to parse rollbacks file: %w", err)
	}

	if rollbacks.Apps == nil {
		rollbacks.Apps = make(map[string]*AppRollbacks)
	}

	return &rollbacks, nil
}

// RecordRollback appends a rollback entry to rollbacks.yaml, commits, and pushes
func (g *GitOps) RecordRollback(app string, entry *RollbackEntry) error {
	repo, err := g.Clone()
	if err != nil {
		return err
	}

	w, err := repo.Worktree()
	if err != nil {
		return err
	}

	// Ensure we're on main
	err = w.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("main"),
	})
	if err != nil {
		return fmt.Errorf("failed to checkout main: %w", err)
	}

	// Read existing rollbacks
	rollbacks, err := g.readRollbacksFromDisk()
	if err != nil {
		return err
	}

	// Update app entry
	appRollbacks, ok := rollbacks.Apps[app]
	if !ok {
		appRollbacks = &AppRollbacks{
			CurrentStatus:  "normal",
			ActiveRollback: nil,
			History:        []*RollbackEntry{},
		}
		rollbacks.Apps[app] = appRollbacks
	}

	appRollbacks.History = append(appRollbacks.History, entry)
	appRollbacks.CurrentStatus = "rollback"
	appRollbacks.ActiveRollback = entry
	rollbacks.GeneratedAt = time.Now().UTC().Format(time.RFC3339)

	// Write back
	path := filepath.Join(g.WorkDir, "rollbacks", "rollbacks.yaml")
	data, err := yaml.Marshal(rollbacks)
	if err != nil {
		return fmt.Errorf("failed to marshal rollbacks: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write rollbacks file: %w", err)
	}

	// Stage, commit, push
	_, err = w.Add("rollbacks/rollbacks.yaml")
	if err != nil {
		return fmt.Errorf("failed to stage rollbacks file: %w", err)
	}

	_, err = w.Commit(fmt.Sprintf("rollback(%s): revert to %.8s - %s", app, entry.TargetRevision, entry.Reason), &git.CommitOptions{
		Author: &object.Signature{
			Name:  entry.InitiatedBy,
			Email: fmt.Sprintf("%s@tinycloud.local", entry.InitiatedBy),
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to commit rollbacks file: %w", err)
	}

	err = repo.Push(&git.PushOptions{
		Auth: &http.BasicAuth{
			Username: g.Username,
			Password: g.Token,
		},
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("failed to push rollbacks commit: %w", err)
	}

	return nil
}

// RecordRestore appends a restore entry and returns to normal state
func (g *GitOps) RecordRestore(app string, entry *RollbackEntry, fastForwardBranch bool) error {
	repo, err := g.Clone()
	if err != nil {
		return err
	}

	w, err := repo.Worktree()
	if err != nil {
		return err
	}

	err = w.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("main"),
	})
	if err != nil {
		return fmt.Errorf("failed to checkout main: %w", err)
	}

	rollbacks, err := g.readRollbacksFromDisk()
	if err != nil {
		return err
	}

	appRollbacks, ok := rollbacks.Apps[app]
	if !ok {
		appRollbacks = &AppRollbacks{
			CurrentStatus:  "normal",
			ActiveRollback: nil,
			History:        []*RollbackEntry{},
		}
		rollbacks.Apps[app] = appRollbacks
	}

	appRollbacks.History = append(appRollbacks.History, entry)
	appRollbacks.CurrentStatus = "normal"
	appRollbacks.ActiveRollback = nil
	rollbacks.GeneratedAt = time.Now().UTC().Format(time.RFC3339)

	path := filepath.Join(g.WorkDir, "rollbacks", "rollbacks.yaml")
	data, err := yaml.Marshal(rollbacks)
	if err != nil {
		return fmt.Errorf("failed to marshal rollbacks: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write rollbacks file: %w", err)
	}

	_, err = w.Add("rollbacks/rollbacks.yaml")
	if err != nil {
		return fmt.Errorf("failed to stage rollbacks file: %w", err)
	}

	_, err = w.Commit(fmt.Sprintf("restore(%s): return to main - %s", app, entry.Reason), &git.CommitOptions{
		Author: &object.Signature{
			Name:  entry.InitiatedBy,
			Email: fmt.Sprintf("%s@tinycloud.local", entry.InitiatedBy),
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to commit rollbacks file: %w", err)
	}

	err = repo.Push(&git.PushOptions{
		Auth: &http.BasicAuth{
			Username: g.Username,
			Password: g.Token,
		},
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("failed to push rollbacks commit: %w", err)
	}

	// Fast-forward rollback branch to main if requested
	if fastForwardBranch {
		branchName := fmt.Sprintf("rollback/%s", app)
		refName := plumbing.NewBranchReferenceName(branchName)

		// Get main HEAD
		headRef, err := repo.Head()
		if err != nil {
			return fmt.Errorf("failed to get HEAD: %w", err)
		}

		// Update rollback branch ref
		newRef := plumbing.NewHashReference(refName, headRef.Hash())
		err = repo.Storer.SetReference(newRef)
		if err != nil {
			return fmt.Errorf("failed to update rollback branch ref: %w", err)
		}

		err = repo.Push(&git.PushOptions{
			RemoteName: "origin",
			RefSpecs:   []config.RefSpec{config.RefSpec(fmt.Sprintf("+%s:%s", refName, refName))},
			Auth: &http.BasicAuth{
				Username: g.Username,
				Password: g.Token,
			},
		})
		if err != nil && err != git.NoErrAlreadyUpToDate {
			return fmt.Errorf("failed to push fast-forwarded branch: %w", err)
		}
	}

	return nil
}

// ValidateSHA checks if a SHA exists in the repo
func (g *GitOps) ValidateSHA(sha string) (bool, error) {
	repo, err := g.Clone()
	if err != nil {
		return false, err
	}

	_, err = repo.CommitObject(plumbing.NewHash(sha))
	if err != nil {
		return false, nil // SHA not found
	}
	return true, nil
}

// memfs import is unused; we use real filesystem
var _ = memfs.New
