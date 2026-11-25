package glossary

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/VolkovDani/project-creator/internal/storage"
)

func TestGlossaryChangeInvalidatesOnlyChangedPairContext(t *testing.T) {
	db, _ := glossaryTestDB(t)
	if _, err := db.Exec(`
	 INSERT INTO projects VALUES ('p','a','P','G','','now','now'),('q','b','Q','G','','now','now');
	 INSERT INTO analysis_runs(id,workspace,project_id,status,generator_profile,embedding_profile,source_signature,sources,lease_until)
	 VALUES ('run','a','p','completed','generator','','','[]',now()),('foreign','b','q','completed','generator','','','[]',now());
	 INSERT INTO analysis_sources(run_id,requirement_id,source)
	 SELECT r.id,s.id,jsonb_build_object('id',s.id,'version',1,'text',s.text)
	 FROM analysis_runs r CROSS JOIN (VALUES
	   ('a','ГП утверждает.'),('b','Заказчик проверяет.'),('c','ГПУ утверждает.'),('d','Срок указан.'),
	   ('e','ГП утверждает.'),('f','ГП утверждает.')
	 ) AS s(id,text);
	 INSERT INTO analysis_pairs(id,run_id,workspace,project_id,position,left_source,right_source,generator_profile,cache_key,status,verdict,selection_reasons,result_origin)
	 SELECT p.id,r.id,r.workspace,r.project_id,p.position,l.source,rr.source,'generator','','completed','compatible',p.reasons::jsonb,p.origin
	 FROM (VALUES
	   ('left','run',0,'a','b','[]','model'),
	   ('right','run',1,'d','e','[]','model'),
	   ('substring','run',2,'b','c','[]','model'),
	   ('exact-reason','run',3,'a','e','["exact_text"]','model'),
	   ('exact-origin','run',4,'a','f','[]','exact_text_v1'),
	   ('other-workspace','foreign',0,'a','b','[]','model')
	 ) p(id,run_id,position,left_id,right_id,reasons,origin)
	 JOIN analysis_runs r ON r.id=p.run_id
	 JOIN analysis_sources l ON l.run_id=r.id AND l.requirement_id=p.left_id
	 JOIN analysis_sources rr ON rr.run_id=r.id AND rr.requirement_id=p.right_id;
	`); err != nil {
		t.Fatal(err)
	}
	change := func(term Term, remove bool, wantStale []string, invalidate bool) Term {
		t.Helper()
		saved, fieldErrors, err := Change(context.Background(), db, "a", "p", term, remove)
		if err != nil || len(fieldErrors) != 0 {
			t.Fatalf("change result: %+v %v", fieldErrors, err)
		}
		var raw []byte
		var stale, keys []string
		if err := db.QueryRow(`SELECT coalesce(jsonb_agg(id ORDER BY id) FILTER (WHERE glossary_stale), '[]') FROM analysis_pairs`).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &stale); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(stale, wantStale) {
			t.Fatalf("stale pairs = %v, want %v", stale, wantStale)
		}
		if err := db.QueryRow(`SELECT to_jsonb(keys) FROM cache_events WHERE workspace='a' ORDER BY seq DESC LIMIT 1`).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &keys); err != nil {
			t.Fatal(err)
		}
		wantKeys := []string{"project:p:glossary"}
		if invalidate {
			wantKeys = append(wantKeys, "project:p:analysis", "project:p:analysis-history")
		}
		slices.Sort(keys)
		slices.Sort(wantKeys)
		if !slices.Equal(keys, wantKeys) {
			t.Fatalf("invalidation keys = %v, want %v", keys, wantKeys)
		}
		return saved
	}
	variant := []Variant{{Text: "ГП", Kind: "abbreviation"}}
	first := change(Term{Name: "Генеральный подрядчик", Variants: variant}, false, []string{"left", "right"}, true)
	second := change(Term{Name: "Генеральный план", Variants: variant}, false, []string{"left", "right"}, false)
	if _, err := db.Exec(`UPDATE analysis_pairs SET glossary_stale=false, glossary_context='[{"ambiguous":true,"spelling":"ГП"}]' WHERE id IN ('left','right')`); err != nil {
		t.Fatal(err)
	}
	first.Definition = "Подрядная организация"
	change(first, false, nil, false)
	change(second, true, []string{"left", "right"}, true)
	change(Term{Name: "Государственная программа", Variants: variant}, false, nil, true)
}

func TestGlossaryChangeLetsEarlierRequirementWriteTakeProjectLock(t *testing.T) {
	db, dsn := glossaryTestDB(t)
	if _, err := db.Exec(`
		INSERT INTO projects VALUES ('p','a','P','G','','now','now');
		INSERT INTO requirements VALUES ('r','a','p','Source',1,'now','now');
	`); err != nil {
		t.Fatal(err)
	}
	editDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { editDB.Close() })
	editDB.SetMaxOpenConns(1)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var editorPID, writerPID int
	if err := editDB.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&editorPID); err != nil {
		t.Fatal(err)
	}

	writer, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Rollback()
	if err := writer.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&writerPID); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.ExecContext(ctx, `SELECT pg_advisory_xact_lock(724183092)`); err != nil {
		t.Fatal(err)
	}

	finished := make(chan error, 1)
	go func() {
		_, fields, err := Change(ctx, editDB, "a", "p", Term{Name: "Term"}, false)
		if len(fields) != 0 {
			t.Errorf("edit field errors: %+v", fields)
		}
		finished <- err
	}()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var blocked bool
		if err := db.QueryRowContext(ctx, `SELECT $1 = ANY(pg_blocking_pids($2))`, writerPID, editorPID).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			break
		}
		select {
		case err := <-finished:
			t.Fatalf("glossary edit finished before waiting for event lock: %v", err)
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}

	if _, err := writer.ExecContext(ctx, `UPDATE requirements SET text='Edited',version=2 WHERE id='r'`); err != nil {
		t.Fatalf("glossary edit blocked requirement write while waiting for event lock: %v", err)
	}
	if err := writer.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func glossaryTestDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("TEST_DATABASE_URL required in CI")
		}
		t.Skip("PostgreSQL integration requires TEST_DATABASE_URL")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	var random [12]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	schema := "glossary_" + hex.EncodeToString(random[:])
	if _, err := admin.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(`DROP SCHEMA ` + schema + ` CASCADE`); err != nil {
			t.Error(err)
		}
	})
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := u.Query()
	query.Set("search_path", schema)
	u.RawQuery = query.Encode()
	db, err := storage.Open(u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db, u.String()
}
