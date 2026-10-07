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
