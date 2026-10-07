package main

import (
	"bytes"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"text/tabwriter"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const schema = `
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

