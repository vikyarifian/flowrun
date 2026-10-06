package db

import (
	"database/sql"
	"os"
	"syscall"
	"time"
)

const Schema = `
CREATE TABLE IF NOT EXISTS flowrun_locks (
    job_key TEXT PRIMARY KEY,
    acquired_at DATETIME NOT NULL,
    pid INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS flowrun_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_key TEXT NOT NULL,
    command TEXT NOT NULL,
    status_code INTEGER NOT NULL,
    started_at DATETIME NOT NULL,
    finished_at DATETIME NOT NULL,
    duration_ms INTEGER NOT NULL,
    output TEXT NOT NULL
);
`

