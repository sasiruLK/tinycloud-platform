package coordinator

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/sasiruLK/tinycloud-platform/internal/build/types"
)

// The original schema had app_name UNIQUE, so an app could be built once and
// never again. These cover the migration off it, including the case that
// matters most: an existing database with real history in it.

func TestMigrationDropsAppNameUniqueAndKeepsRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "builds.db")

	// Build the *old* schema by hand, with the UNIQUE constraint and a row in it.
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	_, err = raw.Exec(`CREATE TABLE build_jobs (
		id TEXT PRIMARY KEY,
		app_name TEXT NOT NULL UNIQUE,
		repo_url TEXT NOT NULL,
		ref TEXT NOT NULL,
		status TEXT NOT NULL,
		replicas INTEGER NOT NULL,
		port INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`)
	if err != nil {
		t.Fatalf("create old schema: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO build_jobs
		(id, app_name, repo_url, ref, status, replicas, port, created_at, updated_at)
		VALUES ('old-1','demo','https://example.test/demo','main','succeeded',1,8080,'2026-01-01T00:00:00Z','2026-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("seed row: %v", err)
	}
	raw.Close()

	// Opening the store runs the migration over that existing database.
	s, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// The existing history must survive a table rebuild. Losing build history
	// to a schema migration would be worse than the constraint.
	got, err := s.GetJobByAppName(ctx, "demo")
	if err != nil {
		t.Fatalf("GetJobByAppName: %v", err)
	}
	if got == nil {
		t.Fatal("existing job was lost by the migration")
	}
	if got.ID != "old-1" || got.RepoURL != "https://example.test/demo" || got.Port != 8080 {
		t.Fatalf("row came back altered: %+v", got)
	}

	// The constraint must actually be gone, not merely unused.
	unique, err := s.hasUniqueAppName(ctx)
	if err != nil {
		t.Fatalf("hasUniqueAppName: %v", err)
	}
	if unique {
		t.Fatal("app_name is still UNIQUE after migration")
	}

	// Which is the point: a second build for the same app can now exist.
	if err := s.CreateJob(ctx, &types.BuildJob{
		ID: "new-1", AppName: "demo", RepoURL: "https://example.test/demo",
		Ref: "main", Status: types.StatusQueued, Replicas: 1, Port: 8080,
	}); err != nil {
		t.Fatalf("second build for the same app should be allowed: %v", err)
	}
}

func TestMigrationIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "builds.db")
	for i := 0; i < 3; i++ {
		s, err := OpenStore(path)
		if err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
		unique, err := s.hasUniqueAppName(context.Background())
		if err != nil {
			t.Fatalf("check %d: %v", i, err)
		}
		if unique {
			t.Fatalf("open %d left app_name unique", i)
		}
		s.Close()
	}
}

// GetJobByAppName is used to decide what a rebuild inherits, so once an app has
// several builds it has to return the newest, not an arbitrary one.
func TestGetJobByAppNameReturnsNewest(t *testing.T) {
	s, err := OpenStore(filepath.Join(t.TempDir(), "builds.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer s.Close()
	ctx := context.Background()

	for _, id := range []string{"b1", "b2", "b3"} {
		if err := s.CreateJob(ctx, &types.BuildJob{
			ID: id, AppName: "demo", RepoURL: "https://example.test/demo",
			Ref: "main", Status: types.StatusSucceeded, Replicas: 2, Port: 3000,
		}); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}

	got, err := s.GetJobByAppName(ctx, "demo")
	if err != nil {
		t.Fatalf("GetJobByAppName: %v", err)
	}
	if got == nil || got.ID != "b3" {
		t.Fatalf("expected newest build b3, got %+v", got)
	}
}
