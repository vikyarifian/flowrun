# flowrun

`flowrun` is a lightweight command-line job wrapper and execution recorder written in Go. It wraps batch script executions (e.g. daily CRON jobs, payroll synchronization, sensor data imports), records stdout/stderr, execution timing, and exit codes into a local SQLite database, and enforces single-instance locks per job key to prevent overlapping executions.

Built for internal corporate servers where multiple background jobs (such as `tarik_absensi`, `proses_pph21`, or monthly ledger rollovers) run via `crontab` and need reliable logging and status checks without setting up complex orchestration platforms.

---

## Features

- **Command Execution & Logging**: Captures `stdout`, `stderr`, execution duration, and exit codes into SQLite.
- **Concurrency Control**: SQLite-backed mutex locks per job key to prevent concurrent duplicate runs.
- **Interactive History**: View recent runs, check job statuses, and inspect failure logs directly from the terminal.
- **Filtering**: Search logs by job key, status (`SUCCESS`, `FAILED`, `RUNNING`), or date range.
- **Log Retention & Purge**: Automated or manual cleanup of historical records past a configured retention window.
- **Zero External Dependencies at Runtime**: Single static binary compiled with embedded SQLite support.

---

## Requirements & Stack

- **Go**: 1.26 or higher
- **Database**: Embedded SQLite (`/var/lib/flowrun/flowrun.db` by default)
- **OS**: Linux / macOS (tested primarily on corporate RedHat/Ubuntu VMs)

---

## Installation

### Build from Source

```bash
git clone https://github.com/internal-it/flowrun.git
cd flowrun
go build -o flowrun main.go
```

### System Deployment

Copy the compiled binary to `/usr/local/bin` and create the default data directory:

```bash
sudo cp flowrun /usr/local/bin/
sudo chmod 755 /usr/local/bin/flowrun
sudo mkdir -p /var/lib/flowrun
sudo chmod 770 /var/lib/flowrun
```

---

## Usage

### 1. Execute a Job with Concurrency Locking

Use `flowrun run` with a unique job key (`--key` or `-k`) followed by `--` and the shell command to execute.

```bash
# Example: Sync attendance data daily
flowrun run --key tarik_absensi -- /opt/scripts/tarik_absensi_fingerprint.sh --site-id 04

# Example: Monthly tax calculation job
flowrun run --key proses_pph21 --python3 /var/www/hris/jobs/hitung_pph21.py
```

If another instance of `tarik_absensi` is already running, `flowrun` exits immediately with status code `42` (Locked) and logs the contention event.

### 2. Standard Crontab Integration

Add `flowrun` wrappers into your server's `/etc/crontab` or user crontab (`crontab -e`):

```cron
# Run attendance pulling every 15 minutes
*/15 * * * * root /usr/local/bin/flowrun run -k tarik_absensi -- /opt/scripts/tarik_absensi.sh >> /var/log/flowrun_cron.log 2>&1

# Daily inventory snapshot at 23:00
0 23 * * * appuser /usr/local/bin/flowrun run -k snapshot_stok -- /opt/bin/generate_stok_harian.sh
```

### 3. Check Job History

Inspect execution history across all jobs or filter by key/status:

```bash
# Show last 20 executions
flowrun history

# Filter by specific job key and status
flowrun history --key tarik_absensi --status FAILED

# Filter by date range
flowrun history --from 2026-03-01 --to 2026-03-31
```

Sample terminal output:

```text
ID    JOB KEY          STATUS     EXIT  DURATION   STARTED AT           
1042  tarik_absensi    SUCCESS    0     4.12s      2026-03-30 08:00:00  
1043  proses_pph21     FAILED     1     12.80s     2026-03-30 08:15:01  
1044  snapshot_stok    LOCKED     42    0.01s      2026-03-30 08:15:05  
```

### 4. Inspect Execution Logs

Retrieve full stdout/stderr details for a specific run ID:

```bash
flowrun inspect --id 1043
```

### 5. Historical Data Purge

Purge logs older than a given number of days:

```bash
# Clean up logs older than 90 days
flowrun purge --days 90
```

---

## Configuration

`flowrun` reads settings from environment variables or command-line flags:

| Variable | Default Value | Description |
|---|---|---|
| `FLOWRUN_DB` | `/var/lib/flowrun/flowrun.db` | Path to SQLite database file |
| `FLOWRUN_RETENTION_DAYS` | `90` | Default retention period for purge operations |
| `FLOWRUN_LOCK_TIMEOUT` | `5s` | Max wait time when requesting SQLite write lock |

Example setting custom database file:

```bash
export FLOWRUN_DB="/data/app/flowrun_custom.db"
flowrun history
```

---

## SQLite Busy Timeout Workaround

When multiple cron entries invoke `flowrun` simultaneously at the top of the minute (e.g. `0 8 * * *`), SQLite can hit `database is locked` errors during initial table initialization or concurrent run status updates.

```go
// SQLite WAL mode supports concurrent readers, but writing connection pool
// needs explicit busy timeout handling in Go database/sql driver.
// Workaround for Go database/sql pooling limitation with CGO/modernc sqlite driver:
// Force single open connection for write transactions to avoid lock escalation delays.
db.SetMaxOpenConctions(1)
```

---

## License

Internal Corporate Utility - Enterprise IT Infrastructure Team.
