package glossary

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/VolkovDani/project-creator/contracts"
	"github.com/VolkovDani/project-creator/internal/project"
	"slices"
	"strings"
	"unicode/utf8"
)

type Reader interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func List(ctx context.Context, db Reader, w, p string) ([]Term, error) {
	return readTerms(ctx, db, " ORDER BY name,id", w, p)
}

// TermsByIDs reads the requested terms in the workspace and project.
func TermsByIDs(ctx context.Context, db Reader, workspace, projectID string, ids []string) ([]Term, error) {
	return readTerms(ctx, db, " AND id = ANY($3::text[]) ORDER BY name,id", workspace, projectID, ids)
}

func TermPage(ctx context.Context, db Reader, w, p, search, name, id string, limit int) ([]Term, error) {
	return readTerms(ctx, db, ` AND (name,id)>($3,$4) AND (strpos(lower(name),lower($5))>0 OR EXISTS (SELECT FROM jsonb_array_elements(variants) v WHERE strpos(lower(v->>'text'),lower($5))>0)) ORDER BY name,id LIMIT $6`, w, p, name, id, strings.TrimSpace(search), limit)
}

func readTerms(ctx context.Context, db Reader, suffix string, args ...any) ([]Term, error) {
	rows, e := db.QueryContext(ctx, `SELECT id,name,definition,meaning_context,variants,version,source_id,source_version,created_at::text,updated_at::text FROM glossary_terms WHERE workspace=$1 AND project_id=$2`+suffix, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var terms []Term
	for rows.Next() {
		var t Term
		var variants []byte
		if e = rows.Scan(&t.ID, &t.Name, &t.Definition, &t.Context, &variants, &t.Version, &t.SourceID, &t.SourceVersion, &t.CreatedAt, &t.UpdatedAt); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(variants, &t.Variants); e != nil {
			return nil, e
		}
		terms = append(terms, t)
	}
	return terms, rows.Err()
}
func invalid(field, code, key string) project.FieldError {
	return project.FieldError{Path: []string{"input", field}, Code: code, MessageKey: key, Params: map[string]int{}}
}

// DraftName applies the glossary name rules to an optional expansion supplied by the editor.
func DraftName(name string) (string, []project.FieldError) {
	if strings.TrimSpace(name) == "" {
		return "", nil
	}
	term := Term{Name: name}
	errs := validate(&term)
	return term.Name, errs
}

func validate(t *Term) []project.FieldError {
	c := contracts.GlossaryRules()
	var errs []project.FieldError
	check := func(field string, s *string, max int, required bool) {
		*s = strings.TrimSpace(*s)
		if !utf8.ValidString(*s) || strings.ContainsRune(*s, 0) {
			errs = append(errs, invalid(field, "INVALID_UTF8", "validation.invalidEncoding"))
		} else if required && *s == "" {
			errs = append(errs, invalid(field, "REQUIRED", "validation.required"))
		} else if utf8.RuneCountInString(*s) > max {
			e := invalid(field, "MAX_LENGTH", "validation.maxLength")
			e.Params["max"] = max
			errs = append(errs, e)
		}
	}
	check("name", &t.Name, c.NameMax, true)
	check("definition", &t.Definition, c.DefinitionMax, false)
	check("context", &t.Context, c.ContextMax, false)
	if len(t.Variants) > c.VariantLimit {
		return append(errs, invalid("variants", "LIMIT", "glossary.invalidVariants"))
	}
	seen := map[string]bool{strings.ToLower(t.Name): true}
	for i := range t.Variants {
		v := &t.Variants[i]
		check("variants", &v.Text, c.VariantMax, true)
		key := strings.ToLower(v.Text)
		if seen[key] || !slices.Contains(c.Kinds, v.Kind) {
			errs = append(errs, invalid("variants", "INVALID_VARIANT", "glossary.invalidVariants"))
		}
		seen[key] = true
	}
	return errs
}

// Change serializes with requirement edits and queue claims. In-flight generations
// lose their attempt token when their matching vocabulary changes.
func Change(ctx context.Context, db *sql.DB, w, p string, t Term, remove bool) (Term, []project.FieldError, error) {
	conflict := []project.FieldError{invalid("version", "CONFLICT", "validation.conflict")}
	if !remove {
		if errs := validate(&t); len(errs) > 0 {
			return Term{}, errs, nil
		}
	}
	tx, e := db.BeginTx(ctx, nil)
	if e != nil {
		return Term{}, nil, e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(724183092)`); e != nil {
		return Term{}, nil, e
	}
	var ok bool
	if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT FROM projects WHERE workspace=$1 AND id=$2)`, w, p).Scan(&ok); e != nil || !ok {
		return Term{}, conflict, e
	}
	existing, e := List(ctx, tx, w, p)
	if e != nil {
		return Term{}, nil, e
	}
	var old *Term
	for i := range existing {
		v := &existing[i]
		if v.ID == t.ID {
			old = v
		} else if !remove && strings.EqualFold(t.Name, v.Name) {
			return Term{}, []project.FieldError{invalid("name", "DUPLICATE", "glossary.duplicate")}, nil
		}
	}
	if t.ID != "" && (old == nil || old.Version != t.Version) || t.ID == "" && (remove || t.Version != 0) {
		return Term{}, conflict, nil
	}
	if !remove && t.SourceID != "" && (old == nil || old.SourceID != t.SourceID || old.SourceVersion != t.SourceVersion) {
		if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT FROM requirements WHERE workspace=$1 AND project_id=$2 AND id=$3 AND version=$4)`, w, p, t.SourceID, t.SourceVersion).Scan(&ok); e != nil || !ok {
			return Term{}, conflict, e
		}
	}
	if t.SourceID == "" && t.SourceVersion != 0 {
		return Term{}, conflict, nil
	}
	variants, e := json.Marshal(t.Variants)
	if e != nil {
		return Term{}, nil, e
	}
	switch {
	case remove:
		_, e = tx.ExecContext(ctx, `DELETE FROM glossary_terms WHERE workspace=$1 AND project_id=$2 AND id=$3`, w, p, t.ID)
	case old == nil:
		var id [16]byte
		if _, e = rand.Read(id[:]); e != nil {
			return Term{}, nil, e
		}
		t.ID = hex.EncodeToString(id[:])
		t.Version = 1
		e = tx.QueryRowContext(ctx, `INSERT INTO glossary_terms(workspace,project_id,id,name,definition,meaning_context,variants,version,source_id,source_version) VALUES($1,$2,$3,$4,$5,$6,$7,1,$8,$9) RETURNING created_at::text,updated_at::text`, w, p, t.ID, t.Name, t.Definition, t.Context, variants, t.SourceID, t.SourceVersion).Scan(&t.CreatedAt, &t.UpdatedAt)
	default:
		t.Version++
		e = tx.QueryRowContext(ctx, `UPDATE glossary_terms SET name=$4,definition=$5,meaning_context=$6,variants=$7,version=$8,source_id=$9,source_version=$10,updated_at=clock_timestamp() WHERE workspace=$1 AND project_id=$2 AND id=$3 RETURNING created_at::text,updated_at::text`, w, p, t.ID, t.Name, t.Definition, t.Context, variants, t.Version, t.SourceID, t.SourceVersion).Scan(&t.CreatedAt, &t.UpdatedAt)
	}
	if e != nil {
		return Term{}, nil, e
	}
	changed := []Term{}
	if old != nil {
		changed = append(changed, *old)
	}
	if !remove {
		changed = append(changed, t)
	}
	if e = requeue(ctx, tx, w, p, changed); e != nil {
		return Term{}, nil, e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO cache_events(id,workspace,keys) VALUES(md5(random()::text||clock_timestamp()::text),$1,ARRAY['project:'||$2||':glossary']);`, w, p)
	if e != nil {
		return Term{}, nil, e
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM cache_events WHERE seq < (SELECT seq FROM cache_events ORDER BY seq DESC OFFSET 999 LIMIT 1)`); e != nil {
		return Term{}, nil, e
	}
	return t, nil, tx.Commit()
}
func requeue(ctx context.Context, tx *sql.Tx, w, p string, changed []Term) error {
	rows, e := tx.QueryContext(ctx, `SELECT r.id,r.text FROM requirements r WHERE r.workspace=$1 AND r.project_id=$2 AND EXISTS (SELECT FROM requirement_summaries s WHERE s.workspace=r.workspace AND s.project_id=r.project_id AND s.requirement_id=r.id AND s.version=r.version)`, w, p)
	if e != nil {
		return e
	}
	changedSpellings := compile(changed)
	var ids []string
	for rows.Next() {
		var id, text string
		if e = rows.Scan(&id, &text); e != nil {
			rows.Close()
			return e
		}
		if len(changedSpellings.matches(text)) > 0 {
			ids = append(ids, id)
		}
	}
	if e = errors.Join(rows.Err(), rows.Close()); e != nil || len(ids) == 0 {
		return e
	}
	_, e = tx.ExecContext(ctx, `UPDATE requirement_summaries s SET status=CASE WHEN s.profile=(SELECT sp.profile FROM summary_profiles sp WHERE sp.workspace=s.workspace) THEN 'queued' ELSE 'stale' END,text='',attempt='',error_code='' FROM requirements r WHERE r.workspace=$1 AND r.project_id=$2 AND r.id=ANY($3) AND s.workspace=r.workspace AND s.project_id=r.project_id AND s.requirement_id=r.id AND s.version=r.version AND s.status<>'cancelled' `, w, p, ids)
	return e
}
