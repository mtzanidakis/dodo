// Package backup implements the `dodo backup` command: an online snapshot of
// the SQLite database, equivalent to `sqlite3 <db> ".backup <dest>"`.
package backup

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/mtzanidakis/dodo/internal/config"

	_ "modernc.org/sqlite"
)

// Run executes `dodo backup -dump <dest>`. It reads the database path the same
// way `serve`/`admin` do (DODO_DATABASE_PATH) and writes a consistent copy to
// dest using `VACUUM INTO`, which is safe against a running server under WAL.
func Run(args []string) int {
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	dump := fs.String("dump", "", "destination path for the backup file (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *dump == "" {
		fmt.Fprintln(os.Stderr, "error: -dump <path> is required")
		fmt.Fprintln(os.Stderr, "usage: dodo backup -dump /data/backup.sqlite")
		return 2
	}
	if _, err := os.Stat(*dump); err == nil {
		fmt.Fprintf(os.Stderr, "error: %s already exists; choose a new path\n", *dump)
		return 1
	}

	cfg, err := config.LoadAdmin()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	d, err := sql.Open("sqlite", cfg.DatabasePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	defer func() { _ = d.Close() }()

	// VACUUM INTO takes a string literal for the destination; quote it safely.
	stmt := "VACUUM INTO '" + strings.ReplaceAll(*dump, "'", "''") + "'"
	if _, err := d.Exec(stmt); err != nil {
		fmt.Fprintln(os.Stderr, "error: backup failed:", err)
		return 1
	}
	fmt.Printf("backup written to %s\n", *dump)
	return 0
}
