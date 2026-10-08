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

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	if len(os.Args) < 2 {
		printHelp()
		os.Exit(1)
	}

	dbPath := getDBPath()
	
	// Workaround for go-sqlite3 driver limitation where busy_timeout cannot be set after opening the database connection.
	// We append it as a DSN parameter to prevent database locked errors during concurrent nightly cron runs (e.g. tagihan vs absensi).
	dsn := dbPath + "?_busy_timeout=5000"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	initDB(db)

	subcommand := os.Args[1]
	switch subcommand {
	case "run":
		runCmd(db)
	case "history":
		historyCmd(db)
	case "purge":
		purgeCmd(db)
	default:
		printHelp()
		os.Exit(1)
	}
}

func getDBPath() string {
	if path := os.Getenv("FLOWRUN_DB"); path != "" {
		if abs, err := filepath.Abs(path); err == nil {
			return abs
		}
		return path
	}
	
	home, err := os.UserHomeDir()
	if err == nil {
		return filepath.Join(home, "flowrun.db")
	}
	return "/tmp/flowrun.db"
}

func initDB(db *sql.DB) {
	_, err := db.Exec(schema)
	if err != nil {
		log.Fatalf("Database initialization failed: %v", err)
	}
}

func runCmd(db *sql.DB) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	keyFlag := fs.String("key", "", "Job lock key")
	cmdFlag := fs.String("cmd", "", "Command to execute")
	fs.Parse(os.Args[2:])

	if *keyFlag == "" || *cmdFlag == "" {
		fmt.Fprintf(os.Stderr, "Error: Both --key and --cmd parameters are required for run action.\n")
		fs.Usage()
		os.Exit(1)
	}

	myPid := os.Getpid()
	ok, err := acquireLock(db, *keyFlag, myPid)
	if err != nil {
		log.Fatalf("Failed during lock acquisition flow: %v", err)
	}
	if !ok {
		fmt.Fprintf(os.Stderr, "Conflict: Job with key '%s' is already being executed by another process. Exiting to prevent duplication.\n", *keyFlag)
		os.Exit(2)
	}
	defer releaseLock(db, *keyFlag, myPid)

	startedAt := time.Now()
	statusCode, outputStr := executeCommand(*cmdFlag)
	finishedAt := time.Now()
	durationMs := finishedAt.Sub(startedAt).Milliseconds()

	_, err = db.Exec(`
		INSERT INTO flowrun_history (job_key, command, status_code, started_at, finished_at, duration_ms, output)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		*keyFlag, *cmdFlag, statusCode, startedAt.Format("2006-01-02 15:04:05"), finishedAt.Format("2006-01-02 15:04:05"), durationMs, outputStr,
	)
	if err != nil {
		log.Printf("Error saving execution log history to database: %v\n", err)
	}
}

func historyCmd(db *sql.DB) {
	fs := flag.NewFlagSet("history", flag.ExitOnError)
	keyFlag := fs.String("key", "", "Filter logs by job key")
	statusFlag := fs.String("status", "", "Filter logs by exit status code")
	fromFlag := fs.String("from", "", "Filter from date (YYYY-MM-DD)")
	toFlag := fs.String("to", "", "Filter to date (YYYY-MM-DD)")
	fs.Parse(os.Args[2:])

	query := "SELECT id, job_key, command, status_code, started_at, finished_at, duration_ms, output FROM flowrun_history WHERE 1=1"
	var args []interface{}

	if *keyFlag != "" {
		query += " AND job_key = ?"
		args = append(args, *keyFlag)
	}
	if *statusFlag != "" {
		statusVal, err := strconv.Atoi(*statusFlag)
		if err != nil {
			log.Fatalf("Invalid status filter value: %v", err)
		}
		query += " AND status_code = ?"
		args = append(args, statusVal)
	}
	if *fromFlag != "" {
		_, err := time.Parse("2006-01-02", *fromFlag)
		if err != nil {
			log.Fatalf("Invalid 'from' date format (must be YYYY-MM-DD): %v", err)
		}
		query += " AND started_at >= ?"
		args = append(args, *fromFlag+" 00:00:00")
	}
	if *toFlag != "" {
		_, err := time.Parse("2006-01-02", *toFlag)
		if err != nil {
			log.Fatalf("Invalid 'to' date format (must be YYYY-MM-DD): %v", err)
		}
		query += " AND started_at <= ?"
		args = append(args, *toFlag+" 23:59:59")
	}

	query += " ORDER BY started_at DESC LIMIT 100"

	rows, err := db.Query(query, args...)
	if err != nil {
		log.Fatalf("Failed executing history query: %v", err)
	}
	defer rows.Close()

	type failedRun struct {
		id     int
		jobKey string
		output string
	}
	var failedLogs []failedRun

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "ID\tJOB KEY\tSTATUS\tSTARTED AT\tDURATION (ms)\tCOMMAND")

	hasRows := false
	for rows.Next() {
		hasRows = true
		var id int
		var jk, cmdStr, startedStr, finishedStr, output string
		var statusCode, durationMs int
		err := rows.Scan(&id, &jk, &cmdStr, &statusCode, &startedStr, &finishedStr, &durationMs, &output)
		if err != nil {
			log.Fatalf("Row scanning failed: %v", err)
		}

		fmt.Fprintf(w, "%d\t%s\t%d\t%s\t%d\t%s\n", id, jk, statusCode, startedStr, durationMs, truncate(cmdStr, 40))

		if statusCode != 0 {
			failedLogs = append(failedLogs, failedRun{id: id, jobKey: jk, output: output})
		}
	}

	if !hasRows {
		fmt.Println("No execution records found matching current criteria.")
		return
	}

	w.Flush()

	if len(failedLogs) > 0 {
		fmt.Println("\n--- RECENT FAILURE LOG DETAILS ---")
		for _, f := range failedLogs {
			fmt.Printf("\n[ID %d] Job Key: %s\n", f.id, f.jobKey)
			fmt.Println("Output:")
			if f.output == "" {
				fmt.Println("(no terminal output logged)")
			} else {
				fmt.Println(f.output)
			}
			fmt.Println("----------------------------------")
		}
	}
}

func purgeCmd(db *sql.DB) {
	fs := flag.NewFlagSet("purge", flag.ExitOnError)
	daysFlag := fs.Int("days", -1, "Retention period in days (older logs will be deleted)")
	fs.Parse(os.Args[2:])

	if *daysFlag < 0 {
		fmt.Fprintf(os.Stderr, "Error: --days parameter is required and must be 0 or greater.\n")
		fs.Usage()
		os.Exit(1)
	}

	cutoff := time.Now().AddDate(0, 0, -(*daysFlag)).Format("2006-01-02 15:04:05")
	result, err := db.Exec("DELETE FROM flowrun_history WHERE started_at < ?", cutoff)
	if err != nil {
		log.Fatalf("Deletion failed: %v", err)
	}

	rowsAffected, _ := result.RowsAffected()
	fmt.Printf("Successfully purged %d log record(s) older than %d days (cutoff date: %s).\n", rowsAffected, *daysFlag, cutoff)
}

func acquireLock(db *sql.DB, jobKey string, pid int) (bool, error) {
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

func releaseLock(db *sql.DB, jobKey string, pid int) {
	_, err := db.Exec("DELETE FROM flowrun_locks WHERE job_key = ? AND pid = ?", jobKey, pid)
	if err != nil {
		log.Printf("Warning: failed to release database lock for '%s': %v\n", jobKey, err)
	}
}

func executeCommand(commandStr string) (int, string) {
	var buf bytes.Buffer
	mwStdout := io.MultiWriter(os.Stdout, &buf)
	mwStderr := io.MultiWriter(os.Stderr, &buf)

	cmd := exec.Command("sh", "-c", commandStr)
	cmd.Stdout = mwStdout
	cmd.Stderr = mwStderr

	err := cmd.Run()
	statusCode := 0
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			statusCode = exitError.ExitCode()
		} else {
			statusCode = -1
			fmt.Fprintf(mwStderr, "\n[flowrun process error] Execution failed: %v\n", err)
		}
	}
	return statusCode, buf.String()
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

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func printHelp() {
	fmt.Println(`flowrun - Manage locks and log execution histories for critical batch processes

Usage:
  flowrun run --key <job_key> --cmd <shell_command>
  flowrun history [--key <job_key>] [--status <status_code>] [--from <YYYY-MM-DD>] [--to <YYYY-MM-DD>]
  flowrun purge --days <retention_period_days>

Examples:
  # Run daily tagihan invoice sync safely without double run conflicts
  flowrun run --key "tagihan-harian" --cmd "php /var/www/html/artisan sync:tagihan"

  # Check history logs for overtime calculation batch script (lembur)
  flowrun history --key "lembur-calc"

  # Delete logs older than 45 days
  flowrun purge --days 45`)
}
