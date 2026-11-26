package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/VolkovDani/project-creator/internal/processgraphcache"
	"github.com/VolkovDani/project-creator/internal/storage"
)

func runClearProcessGraphCache(ctx context.Context, args []string, getenv func(string) string, output io.Writer) error {
	flags := flag.NewFlagSet("clear-process-graph-cache", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	confirmed := flags.Bool("yes", false, "delete the cached results")
	var projectID string
	flags.Func("project-id", "limit the operation to this project ID", func(value string) error {
		if strings.TrimSpace(value) == "" {
			return errors.New("project ID must not be empty")
		}
		projectID = value
		return nil
	})
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return errors.New("usage: clear-process-graph-cache [--project-id ID] [--yes]")
	}

	db, err := storage.Open(getenv("PROJECT_CREATOR_DATABASE_URL"))
	if err != nil {
		return errors.New("cannot open the configured PostgreSQL database")
	}
	defer db.Close()

	operation := processgraphcache.Count
	if *confirmed {
		operation = processgraphcache.Clear
	}
	count, err := operation(ctx, db, projectID)
	if err != nil {
		return errors.New("process graph cache database operation failed")
	}
	_, err = fmt.Fprintln(output, count)
	return err
}
