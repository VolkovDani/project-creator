package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/VolkovDani/project-creator/internal/storage"
)

type aiCacheScope struct {
	classification bool
	axis           string
	processGraphs  bool
}

func runClearAICache(ctx context.Context, args []string, getenv func(string) string, output io.Writer) error {
	flags := flag.NewFlagSet("clear-ai-cache", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	confirmed := flags.Bool("yes", false, "delete the cached results")
	var scope aiCacheScope
	selectScope := func(value aiCacheScope) error {
		if scope != (aiCacheScope{}) || value == (aiCacheScope{}) {
			return errors.New("select exactly one valid cache section, group or --all")
		}
		scope = value
		return nil
	}
	flags.Func("section", "select one cache section", func(value string) error {
		return selectScope(map[string]aiCacheScope{
			"classification-meaning": {classification: true, axis: "meaning"},
			"classification-form":    {classification: true, axis: "form"},
			"process-graphs":         {processGraphs: true},
		}[value])
	})
	flags.Func("group", "select one cache group", func(value string) error {
		return selectScope(map[string]aiCacheScope{
			"classification": {classification: true},
			"diagrams":       {processGraphs: true},
		}[value])
	})
	flags.BoolFunc("all", "select every cache section", func(value string) error {
		if value != "true" {
			return errors.New("--all requires an affirmative selection")
		}
		return selectScope(aiCacheScope{classification: true, processGraphs: true})
	})
	var projectID string
	flags.Func("project-id", "limit the operation to this project ID", func(value string) error {
		if strings.TrimSpace(value) == "" {
			return errors.New("project ID must not be empty")
		}
		projectID = value
		return nil
	})
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || scope == (aiCacheScope{}) {
		return errors.New("usage: clear-ai-cache (--section classification-meaning|classification-form|process-graphs | --group classification|diagrams | --all) [--project-id ID] [--yes]")
	}

	db, err := storage.Open(getenv("PROJECT_CREATOR_DATABASE_URL"))
	if err != nil {
		return errors.New("cannot open the configured PostgreSQL database")
	}
	defer db.Close()
	count, err := operateAICaches(ctx, db, scope, projectID, *confirmed)
	if err != nil {
		return errors.New("AI cache database operation failed")
	}
	_, err = fmt.Fprintln(output, count)
	return err
}

func operateAICaches(ctx context.Context, db *sql.DB, scope aiCacheScope, projectID string, confirmed bool) (int64, error) {
	options := &sql.TxOptions{}
	if !confirmed {
		options.ReadOnly = true
		options.Isolation = sql.LevelRepeatableRead
	}
	tx, err := db.BeginTx(ctx, options)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var count int64
	if scope.classification {
		rows, err := operateAICache(ctx, tx, confirmed,
			`SELECT count(*) FROM classification_explanation_cache WHERE ($1='' OR project_id=$1) AND ($2='' OR axis=$2)`,
			`DELETE FROM classification_explanation_cache WHERE ($1='' OR project_id=$1) AND ($2='' OR axis=$2)`,
			projectID, scope.axis)
		if err != nil {
			return 0, err
		}
		count += rows
	}
	if scope.processGraphs {
		rows, err := operateAICache(ctx, tx, confirmed,
			`SELECT count(*) FROM process_graph_cache WHERE $1='' OR project_id=$1`,
			`DELETE FROM process_graph_cache WHERE $1='' OR project_id=$1`, projectID)
		if err != nil {
			return 0, err
		}
		count += rows
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return count, nil
}

func operateAICache(ctx context.Context, tx *sql.Tx, confirmed bool, previewSQL, deleteSQL string, args ...any) (int64, error) {
	if confirmed {
		result, err := tx.ExecContext(ctx, deleteSQL, args...)
		if err != nil {
			return 0, err
		}
		return result.RowsAffected()
	}
	var count int64
	err := tx.QueryRowContext(ctx, previewSQL, args...).Scan(&count)
	return count, err
}
