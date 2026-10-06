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
