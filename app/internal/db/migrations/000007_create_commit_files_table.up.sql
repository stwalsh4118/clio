CREATE TABLE IF NOT EXISTS commit_files (
    id TEXT PRIMARY KEY,
    commit_id TEXT NOT NULL,
    file_path TEXT NOT NULL,
    lines_added INTEGER NOT NULL DEFAULT 0,
    lines_removed INTEGER NOT NULL DEFAULT 0,
    diff TEXT,
    created_at TIMESTAMP NOT NULL,
    FOREIGN KEY (commit_id) REFERENCES commits(id) ON DELETE CASCADE,
    UNIQUE (commit_id, file_path)
);

CREATE INDEX IF NOT EXISTS idx_commit_files_commit_id ON commit_files(commit_id);
CREATE INDEX IF NOT EXISTS idx_commit_files_file_path ON commit_files(file_path);

