package coordinator

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/sasiruLK/tinycloud-platform/internal/build/types"
	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func OpenStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS build_jobs (
			id TEXT PRIMARY KEY,
			app_name TEXT NOT NULL UNIQUE,
			repo_url TEXT NOT NULL,
			ref TEXT NOT NULL,
			commit_sha TEXT NOT NULL DEFAULT '',
			framework TEXT NOT NULL DEFAULT '',
			image TEXT NOT NULL DEFAULT '',
			tag TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			attempts INTEGER NOT NULL DEFAULT 0,
			replicas INTEGER NOT NULL,
			port INTEGER NOT NULL,
			env_json TEXT NOT NULL DEFAULT '{}',
			gitops_commit_sha TEXT NOT NULL DEFAULT '',
			gitops_path TEXT NOT NULL DEFAULT '',
			deploy_status TEXT NOT NULL DEFAULT '',
			argo_sync_status TEXT NOT NULL DEFAULT '',
			argo_health TEXT NOT NULL DEFAULT '',
			app_url TEXT NOT NULL DEFAULT '',
			verification_error TEXT NOT NULL DEFAULT '',
			error TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			started_at TEXT,
			finished_at TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS build_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			job_id TEXT NOT NULL,
			sequence INTEGER NOT NULL,
			timestamp TEXT NOT NULL,
			stream TEXT NOT NULL,
			message TEXT NOT NULL,
			FOREIGN KEY(job_id) REFERENCES build_jobs(id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_build_jobs_status_created ON build_jobs(status, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_build_logs_job_sequence ON build_logs(job_id, sequence)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	if err := s.ensureBuildJobColumns(ctx); err != nil {
		return err
	}
	return nil
}

func (s *Store) ensureBuildJobColumns(ctx context.Context) error {
	columns, err := s.buildJobColumns(ctx)
	if err != nil {
		return err
	}
	additions := map[string]string{
		"gitops_commit_sha":  "TEXT NOT NULL DEFAULT ''",
		"gitops_path":        "TEXT NOT NULL DEFAULT ''",
		"deploy_status":      "TEXT NOT NULL DEFAULT ''",
		"argo_sync_status":   "TEXT NOT NULL DEFAULT ''",
		"argo_health":        "TEXT NOT NULL DEFAULT ''",
		"app_url":            "TEXT NOT NULL DEFAULT ''",
		"verification_error": "TEXT NOT NULL DEFAULT ''",
	}
	for name, ddl := range additions {
		if columns[name] {
			continue
		}
		if _, err := s.db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE build_jobs ADD COLUMN %s %s", name, ddl)); err != nil {
			return fmt.Errorf("failed to add build_jobs.%s: %w", name, err)
		}
	}
	return nil
}

func (s *Store) buildJobColumns(ctx context.Context) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(build_jobs)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return nil, err
		}
		columns[name] = true
	}
	return columns, rows.Err()
}

func (s *Store) GetJobByAppName(ctx context.Context, appName string) (*types.BuildJob, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, app_name, repo_url, ref, commit_sha, framework, image, tag,
		status, attempts, replicas, port, env_json, gitops_commit_sha, gitops_path, deploy_status,
		argo_sync_status, argo_health, app_url, verification_error, error, created_at, updated_at, started_at, finished_at
		FROM build_jobs WHERE app_name = ?`, appName)
	job, err := scanJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return job, err
}

func (s *Store) DeleteJob(ctx context.Context, id string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM build_logs WHERE job_id = ?`, id); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM build_jobs WHERE id = ?`, id)
	return err
}

func (s *Store) CreateJob(ctx context.Context, job *types.BuildJob) error {
	envJSON, err := json.Marshal(job.Env)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	job.CreatedAt = now
	job.UpdatedAt = now
	_, err = s.db.ExecContext(ctx, `INSERT INTO build_jobs
		(id, app_name, repo_url, ref, status, attempts, replicas, port, env_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		job.ID, job.AppName, job.RepoURL, job.Ref, job.Status, job.Attempts, job.Replicas, job.Port,
		string(envJSON), formatTime(job.CreatedAt), formatTime(job.UpdatedAt),
	)
	return err
}

func (s *Store) GetJob(ctx context.Context, id string) (*types.BuildJob, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, app_name, repo_url, ref, commit_sha, framework, image, tag,
		status, attempts, replicas, port, env_json, gitops_commit_sha, gitops_path, deploy_status,
		argo_sync_status, argo_health, app_url, verification_error, error, created_at, updated_at, started_at, finished_at
		FROM build_jobs WHERE id = ?`, id)
	return scanJob(row)
}

func (s *Store) ClaimNextQueuedJob(ctx context.Context, maxAttempts int) (*types.BuildJob, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	row := tx.QueryRowContext(ctx, `SELECT id, app_name, repo_url, ref, commit_sha, framework, image, tag,
		status, attempts, replicas, port, env_json, gitops_commit_sha, gitops_path, deploy_status,
		argo_sync_status, argo_health, app_url, verification_error, error, created_at, updated_at, started_at, finished_at
		FROM build_jobs WHERE status = ? AND attempts < ? ORDER BY created_at ASC LIMIT 1`,
		types.StatusQueued, maxAttempts,
	)
	job, err := scanJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	_, err = tx.ExecContext(ctx, `UPDATE build_jobs SET status = ?, attempts = attempts + 1,
		updated_at = ?, started_at = COALESCE(started_at, ?) WHERE id = ? AND status = ?`,
		types.StatusRunning, formatTime(now), formatTime(now), job.ID, types.StatusQueued,
	)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetJob(ctx, job.ID)
}

func (s *Store) AppendLog(ctx context.Context, jobID, stream, message string) error {
	var next int64
	row := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence), 0) + 1 FROM build_logs WHERE job_id = ?`, jobID)
	if err := row.Scan(&next); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO build_logs (job_id, sequence, timestamp, stream, message)
		VALUES (?, ?, ?, ?, ?)`, jobID, next, formatTime(time.Now().UTC()), stream, message)
	return err
}

func (s *Store) ListLogs(ctx context.Context, jobID string, after int64) ([]types.BuildLogLine, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT sequence, timestamp, stream, message FROM build_logs
		WHERE job_id = ? AND sequence > ? ORDER BY sequence ASC`, jobID, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var lines []types.BuildLogLine
	for rows.Next() {
		var line types.BuildLogLine
		var ts string
		if err := rows.Scan(&line.Sequence, &ts, &line.Stream, &line.Message); err != nil {
			return nil, err
		}
		line.Timestamp, _ = parseTime(ts)
		lines = append(lines, line)
	}
	return lines, rows.Err()
}

func (s *Store) UpdateRunnerStatus(ctx context.Context, jobID string, req types.RunnerStatusRequest) error {
	now := time.Now().UTC()
	switch req.Status {
	case types.StatusRunning:
		_, err := s.db.ExecContext(ctx, `UPDATE build_jobs SET commit_sha = ?, framework = ?,
			deploy_status = '', argo_sync_status = '', argo_health = '', verification_error = '',
			updated_at = ? WHERE id = ?`, req.CommitSHA, req.Framework, formatTime(now), jobID)
		return err
	case types.StatusSucceeded:
		_, err := s.db.ExecContext(ctx, `UPDATE build_jobs SET status = ?, commit_sha = ?, framework = ?,
			image = ?, tag = ?, gitops_commit_sha = ?, gitops_path = ?, deploy_status = ?,
			app_url = ?, verification_error = ?, error = '', updated_at = ?, finished_at = ? WHERE id = ?`,
			types.StatusSucceeded, req.CommitSHA, req.Framework, req.Image, req.Tag, req.GitOpsCommitSHA,
			req.GitOpsPath, req.DeployStatus, req.AppURL, req.VerificationError,
			formatTime(now), formatTime(now), jobID)
		return err
	case types.StatusFailed:
		_, err := s.db.ExecContext(ctx, `UPDATE build_jobs SET status = ?, error = ?, updated_at = ?,
			finished_at = ? WHERE id = ?`, types.StatusFailed, req.Error, formatTime(now), formatTime(now), jobID)
		return err
	default:
		return fmt.Errorf("unsupported status %q", req.Status)
	}
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanJob(row rowScanner) (*types.BuildJob, error) {
	var job types.BuildJob
	var envJSON, created, updated string
	var started, finished sql.NullString
	if err := row.Scan(&job.ID, &job.AppName, &job.RepoURL, &job.Ref, &job.CommitSHA, &job.Framework,
		&job.Image, &job.Tag, &job.Status, &job.Attempts, &job.Replicas, &job.Port, &envJSON,
		&job.GitOpsCommitSHA, &job.GitOpsPath, &job.DeployStatus, &job.ArgoSyncStatus, &job.ArgoHealth,
		&job.AppURL, &job.VerificationError, &job.Error,
		&created, &updated, &started, &finished); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(envJSON), &job.Env)
	job.CreatedAt, _ = parseTime(created)
	job.UpdatedAt, _ = parseTime(updated)
	if started.Valid {
		t, _ := parseTime(started.String)
		job.StartedAt = &t
	}
	if finished.Valid {
		t, _ := parseTime(finished.String)
		job.FinishedAt = &t
	}
	return &job, nil
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(v string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, v)
}
