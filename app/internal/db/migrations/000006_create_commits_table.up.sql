CREATE TABLE IF NOT EXISTS commits (
    id TEXT PRIMARY KEY,
    session_id TEXT,
    repository_path TEXT NOT NULL,
    repository_name TEXT NOT NULL,
    hash TEXT NOT NULL,
    message TEXT NOT NULL,
    author_name TEXT NOT NULL,
    author_email TEXT NOT NULL,
    timestamp TIMESTAMP NOT NULL,
    branch TEXT NOT NULL,
    is_merge INTEGER NOT NULL DEFAULT 0,
    parent_hashes TEXT,
    full_diff TEXT,
    diff_truncated INTEGER NOT NULL DEFAULT 0,
    diff_truncated_at INTEGER,
    correlation_type TEXT,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_commits_session_id ON commits(session_id);
CREATE INDEX IF NOT EXISTS idx_commits_timestamp ON commits(timestamp);
CREATE INDEX IF NOT EXISTS idx_commits_repository_path ON commits(repository_path);
CREATE INDEX IF NOT EXISTS idx_commits_hash ON commits(hash);

