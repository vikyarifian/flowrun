package history

import (
	"database/sql"
	"fmt"
	"io"
	"strconv"
	"text/tabwriter"
	"time"
)

// RunRecord represents an individual batch execution log entry stored in flowrun_history.
type RunRecord struct {
	ID         int
	JobKey     string
	Command    string
	StatusCode int
	StartedAt  string
	FinishedAt string
	DurationMs int64
	Output     string
}

// GetHistory retrieves historic run logs from the database based on optional filters.
func GetHistory(db *sql.DB, keyFilter, statusFilter, fromDate, toDate string) ([]RunRecord, error) {
	query := "SELECT id, job_key, command, status_code, started_at, finished_at, duration_ms, output FROM flowrun_history WHERE 1=1"
	var args []interface{}

	if keyFilter != "" {
		query += " AND job_key = ?"
		args = append(args, keyFilter)
	}
	if statusFilter != "" {
		statusVal, err := strconv.Atoi(statusFilter)
		if err != nil {
			return nil, fmt.Errorf("invalid status value: %w", err)
		}
		query += " AND status_code = ?"
		args = append(args, statusVal)
	}
	if fromDate != "" {
		_, err := time.Parse("2006-01-02", fromDate)
		if err != nil {
			return nil, fmt.Errorf("invalid 'from' date format: %w", err)
		}
		query += " AND started_at >= ?"
		args = append(args, fromDate+" 00:00:00")
	}
	if toDate != "" {
		_, err := time.Parse("2006-01-02", toDate)
		if err != nil {
			return nil, fmt.Errorf("invalid 'to' date format: %w", err)
		}
		query += " AND started_at <= ?"
		args = append(args, toDate+" 23:59:59")
	}

	query += " ORDER BY started_at DESC LIMIT 100"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []RunRecord
	for rows.Next() {
		var r RunRecord
		err := rows.Scan(&r.ID, &r.JobKey, &r.Command, &r.StatusCode, &r.StartedAt, &r.FinishedAt, &r.DurationMs, &r.Output)
		if err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, nil
}

// FormatAndPrintHistory displays the historical runs in a clean table format on the terminal.
func FormatAndPrintHistory(w io.Writer, records []RunRecord) {
	if len(records) == 0 {
		fmt.Fprintln(w, "No execution records found matching current criteria.")
		return
	}

	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	fmt.Fprintln(tw, "ID\tJOB KEY\tSTATUS\tSTARTED AT\tDURATION (ms)\tCOMMAND")

	type failedRun struct {
		id     int
		jobKey string
		output string
	}
	var failedLogs []failedRun

	for _, r := range records {
		cmdStr := r.Command
		if len(cmdStr) > 40 {
			cmdStr = cmdStr[:37] + "..."
		}
		fmt.Fprintf(tw, "%d\t%s\t%d\t%s\t%d\t%s\n", r.ID, r.JobKey, r.StatusCode, r.StartedAt, r.DurationMs, cmdStr)

		if r.StatusCode != 0 {
			failedLogs = append(failedLogs, failedRun{id: r.ID, jobKey: r.JobKey, output: r.Output})
		}
	}
	tw.Flush()

	if len(failedLogs) > 0 {
		fmt.Fprintln(w, "\n--- RECENT FAILURE LOG DETAILS ---")
		for _, f := range failedLogs {
			fmt.Fprintf(w, "\n[ID %d] Job Key: %s\n", f.id, f.jobKey)
			fmt.Fprintln(w, "Output:")
			if f.output == "" {
				fmt.Fprintln(w, "(no terminal output logged)")
			} else {
				fmt.Fprintln(w, f.output)
			}
			fmt.Fprintln(w, "----------------------------------")
		}
	}
}

// PurgeOldRecords deletes historical execution records older than the specified retention days.
func PurgeOldRecords(db *sql.DB, days int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -days).Format("2006-01-02 15:04:05")
	result, err := db.Exec("DELETE FROM flowrun_history WHERE started_at < ?", cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
