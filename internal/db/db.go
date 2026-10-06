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

func AcquireLock(db *sql.DB, jobKey string, pid int) (bool, error) {
	tx, err := db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	var existingPid int
	var acquiredAt string
	err = tx.QueryRow("SELECT pid, acquired_at FROM flowrun_locks WHERE job_key = ?", jobKey).Scan(&existingPid, &acquiredAt)
	if err == nil {
		if isProcessAlive(existingPid) {
			return false, nil
		}
		_, err = tx.Exec("DELETE FROM flowrun_locks WHERE job_key = ?", jobKey)
		if err != nil {
			return false, err
		}
	} else if err != sql.ErrNoRows {
		return false, err
	}

	_, err = tx.Exec("INSERT INTO flowrun_locks (job_key, acquired_at, pid) VALUES (?, ?, ?)", jobKey, time.Now().Format("2006-01-02 15:04:05"), pid)
	if err != nil {
		return false, err
	}

	err = tx.Commit()
	if err != nil {
		return false, err
	}
	return true, nil
}

func ReleaseLock(db *sql.DB, jobKey string, pid int) error {
	_, err := db.Exec("DELETE FROM flowrun_locks WHERE job_key = ? AND pid = ?", jobKey, pid)
	return err
}

func isProcessAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	
	err = process.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	if err == syscall.EPERM {
		return true
	}
	return false
}
