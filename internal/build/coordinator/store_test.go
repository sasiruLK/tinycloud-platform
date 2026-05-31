package coordinator

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/sasiruLK/tinycloud-platform/internal/build/types"
	"github.com/stretchr/testify/require"
)

func TestStoreCreateClaimUpdateAndLogs(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "builds.db"))
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	job := &types.BuildJob{
		ID: "job-1", AppName: "my-app", RepoURL: "https://github.com/user/repo", Ref: "main",
		Status: types.StatusQueued, Replicas: 1, Port: 8080,
	}
	require.NoError(t, store.CreateJob(ctx, job))

	claimed, err := store.ClaimNextQueuedJob(ctx, 2)
	require.NoError(t, err)
	require.Equal(t, "job-1", claimed.ID)
	require.Equal(t, types.StatusRunning, claimed.Status)
	require.Equal(t, 1, claimed.Attempts)

	require.NoError(t, store.AppendLog(ctx, job.ID, "stdout", "hello"))
	logs, err := store.ListLogs(ctx, job.ID, 0)
	require.NoError(t, err)
	require.Len(t, logs, 1)
	require.Equal(t, int64(1), logs[0].Sequence)

	require.NoError(t, store.UpdateRunnerStatus(ctx, job.ID, types.RunnerStatusRequest{
		Status: types.StatusSucceeded, CommitSHA: "abc", Framework: "go", Image: "ghcr.io/a/b", Tag: "abc",
	}))
	done, err := store.GetJob(ctx, job.ID)
	require.NoError(t, err)
	require.Equal(t, types.StatusSucceeded, done.Status)
	require.Equal(t, "ghcr.io/a/b", done.Image)
}

func TestStoreDeleteJobAllowsRetry(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "builds.db"))
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	job := &types.BuildJob{
		ID: "job-1", AppName: "my-app", RepoURL: "https://github.com/user/repo", Ref: "main",
		Status: types.StatusFailed, Replicas: 1, Port: 8080,
	}
	require.NoError(t, store.CreateJob(ctx, job))
	require.NoError(t, store.AppendLog(ctx, job.ID, "stderr", "boom"))

	found, err := store.GetJobByAppName(ctx, "my-app")
	require.NoError(t, err)
	require.Equal(t, "job-1", found.ID)

	require.NoError(t, store.DeleteJob(ctx, job.ID))
	found, err = store.GetJobByAppName(ctx, "my-app")
	require.NoError(t, err)
	require.Nil(t, found)

	retry := &types.BuildJob{
		ID: "job-2", AppName: "my-app", RepoURL: "https://github.com/user/repo", Ref: "main",
		Status: types.StatusQueued, Replicas: 1, Port: 8080,
	}
	require.NoError(t, store.CreateJob(ctx, retry))
}
