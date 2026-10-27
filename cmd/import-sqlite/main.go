package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/VolkovDani/project-creator/internal/legacy"
	"github.com/VolkovDani/project-creator/internal/storage"
	_ "github.com/mattn/go-sqlite3"
)

func readOnly(path string) (*sql.DB, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	return sql.Open("sqlite3", (&url.URL{Scheme: "file", Path: abs, RawQuery: "mode=ro"}).String())
}

func run() error {
	source := flag.String("source", "", "SQLite database (stop application writers first)")
	backup := flag.String("backup", "", "new backup file; must not already exist")
	flag.Parse()
	if *source == "" || *backup == "" {
		return fmt.Errorf("--source and --backup are required")
	}
	dsn := os.Getenv("PROJECT_CREATOR_DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("PROJECT_CREATOR_DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	src, err := readOnly(*source)
	if err != nil {
		return err
	}
	defer src.Close()
	if err = legacy.Backup(ctx, src, *backup); err != nil {
		return err
	}
	// Read the snapshot we backed up, not a source that may change after backup.
	snapshot, err := readOnly(*backup)
	if err != nil {
		return err
	}
	defer snapshot.Close()
	dst, err := storage.Open(dsn)
	if err != nil {
		return err
	}
	defer dst.Close()
	counts, err := legacy.Import(ctx, snapshot, dst)
	if err != nil {
		return err
	}
	fmt.Printf("Imported and verified %d projects and %d requirements. Backup retained: %s\n", counts.Projects, counts.Requirements, *backup)
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Import failed:", err)
		os.Exit(1)
	}
}
