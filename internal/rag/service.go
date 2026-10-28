package rag

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrNotFound = errors.New("project or requirement not found")
var ErrInvalid = errors.New("invalid search or pagination")
var ErrNotConfigured = errors.New("local embedding provider not configured")
var errStorage = errors.New("index storage unavailable")

type State struct {
	RequirementID              string
	Version                    int
	Status, Profile, ErrorCode string
}
type Hit struct {
	RequirementID, ProjectID, Text, SourceText string
	Version, Start, End                        int
	Score                                      float64
}
type Service struct {
	db       *sql.DB
	embedder Embedder
}

func New(db *sql.DB, embedder Embedder) *Service { return &Service{db: db, embedder: embedder} }
func (s *Service) authorize(ctx context.Context, workspace, project string) error {
	var exists bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM projects WHERE workspace=$1 AND id=$2)`, workspace, project).Scan(&exists); err != nil {
		return errStorage
	}
	if !exists {
		return ErrNotFound
	}
	return nil
}
func (s *Service) profile() string {
	if s.embedder == nil {
		return ""
	}
	return s.embedder.Profile()
}

func (s *Service) Status(ctx context.Context, workspace, project string, limit, offset int) ([]State, error) {
	if err := s.authorize(ctx, workspace, project); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 100 || offset < 0 {
		return nil, ErrInvalid
	}
	profile := s.profile()
	rows, err := s.db.QueryContext(ctx, `SELECT r.id,r.version,coalesce(i.status,'pending'),coalesce(i.error_code,'') FROM requirements r LEFT JOIN requirement_index i ON i.workspace=r.workspace AND i.project_id=r.project_id AND i.requirement_id=r.id AND i.version=r.version AND i.profile=$3 WHERE r.workspace=$1 AND r.project_id=$2 ORDER BY r.created_at,r.id LIMIT $4 OFFSET $5`, workspace, project, profile, limit, offset)
	if err != nil {
		return nil, errStorage
	}
	defer rows.Close()
	out := []State{}
	for rows.Next() {
		state := State{Profile: profile}
		if err = rows.Scan(&state.RequirementID, &state.Version, &state.Status, &state.ErrorCode); err != nil {
			return nil, errStorage
		}
		if profile == "" {
			state.ErrorCode = "NOT_CONFIGURED"
		}
		out = append(out, state)
	}
	if rows.Err() != nil {
		return nil, errStorage
	}
	return out, nil
}

// Index claims a short durable lease, calls only the configured local provider
// outside the transaction, and publishes results only for the claimed version.
func (s *Service) Index(ctx context.Context, workspace, project, requirement string) (State, error) {
	state := State{RequirementID: requirement, Status: "pending", Profile: s.profile()}
	if err := s.authorize(ctx, workspace, project); err != nil {
		return state, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return state, errStorage
	}
	defer tx.Rollback()
	var source string
	if err = tx.QueryRowContext(ctx, `SELECT text,version FROM requirements WHERE workspace=$1 AND project_id=$2 AND id=$3 FOR UPDATE`, workspace, project, requirement).Scan(&source, &state.Version); err == sql.ErrNoRows {
		return state, ErrNotFound
	} else if err != nil {
		return state, errStorage
	}
	if s.embedder == nil || state.Profile == "" {
		state.ErrorCode = "NOT_CONFIGURED"
		return state, ErrNotConfigured
	}
	var status, errorCode string
	var version int
	var active bool
	err = tx.QueryRowContext(ctx, `SELECT status,error_code,version,lease_until>clock_timestamp() FROM requirement_index WHERE workspace=$1 AND project_id=$2 AND requirement_id=$3 AND profile=$4 FOR UPDATE`, workspace, project, requirement, state.Profile).Scan(&status, &errorCode, &version, &active)
	if err != nil && err != sql.ErrNoRows {
		return state, errStorage
	}
	if err == nil && version == state.Version && (status == "ready" || (status == "pending" && active)) {
		state.Status = status
		state.ErrorCode = errorCode
		return state, nil
	}
	var bytes [16]byte
	if _, err = rand.Read(bytes[:]); err != nil {
		return state, errors.New("index attempt unavailable")
	}
	attempt := hex.EncodeToString(bytes[:])
	if _, err = tx.ExecContext(ctx, `INSERT INTO requirement_index(workspace,project_id,requirement_id,profile,version,status,attempt,lease_until,error_code) VALUES($1,$2,$3,$4,$5,'pending',$6,clock_timestamp()+interval '2 minutes','') ON CONFLICT(workspace,project_id,requirement_id,profile) DO UPDATE SET version=excluded.version,status='pending',attempt=excluded.attempt,lease_until=excluded.lease_until,error_code=''`, workspace, project, requirement, state.Profile, state.Version, attempt); err != nil {
		return state, errStorage
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM requirement_chunks WHERE workspace=$1 AND project_id=$2 AND requirement_id=$3 AND profile=$4`, workspace, project, requirement, state.Profile); err != nil {
		return state, errStorage
	}
	if err = tx.Commit(); err != nil {
		return state, errStorage
	}
	chunks := Split(source)
	embeddings := make([][]float32, 0, len(chunks))
	embedCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	code := ""
	for start := 0; start < len(chunks); start += 32 {
		end := min(start+32, len(chunks))
		texts := make([]string, end-start)
		for i := start; i < end; i++ {
			texts[i-start] = chunks[i].Text
		}
		vectors, e := s.embedder.Embed(embedCtx, texts)
		if e != nil || len(vectors) != len(texts) {
			code = "EMBEDDING_FAILED"
			break
		}
		for _, vector := range vectors {
			if _, e = vectorLiteral(vector); e != nil {
				code = "INVALID_EMBEDDING"
				break
			}
		}
		if code != "" {
			break
		}
		embeddings = append(embeddings, vectors...)
	}
	if len(chunks) == 0 {
		code = "EMPTY_SOURCE"
	}
	// Persist failure even when an HTTP caller disconnected; a bounded independent
	// context leaves retry available instead of waiting for the lease to expire.
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer finishCancel()
	return s.finish(finishCtx, workspace, project, state, attempt, chunks, embeddings, code)
}

func (s *Service) finish(ctx context.Context, workspace, project string, state State, attempt string, chunks []Chunk, vectors [][]float32, code string) (State, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return state, errStorage
	}
	defer tx.Rollback()
	var current int
	if err = tx.QueryRowContext(ctx, `SELECT version FROM requirements WHERE workspace=$1 AND project_id=$2 AND id=$3 FOR UPDATE`, workspace, project, state.RequirementID).Scan(&current); err == sql.ErrNoRows {
		return state, ErrNotFound
	} else if err != nil {
		return state, errStorage
	}
	var owner string
	err = tx.QueryRowContext(ctx, `SELECT attempt FROM requirement_index WHERE workspace=$1 AND project_id=$2 AND requirement_id=$3 AND profile=$4 FOR UPDATE`, workspace, project, state.RequirementID, state.Profile).Scan(&owner)
	if err != nil && err != sql.ErrNoRows {
		return state, errStorage
	}
	if current != state.Version || owner != attempt {
		state.Version = current
		state.Status = "pending"
		state.ErrorCode = ""
		return state, nil
	}
	if code == "" {
		for ordinal, chunk := range chunks {
			vector, e := vectorLiteral(vectors[ordinal])
			if e != nil {
				return state, errors.New("invalid embedding")
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO requirement_chunks(workspace,project_id,requirement_id,profile,version,ordinal,start_offset,end_offset,text,embedding) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::public.vector)`, workspace, project, state.RequirementID, state.Profile, state.Version, ordinal, chunk.Start, chunk.End, chunk.Text, vector); err != nil {
				return state, errStorage
			}
		}
		state.Status = "ready"
	} else {
		state.Status = "failed"
		state.ErrorCode = code
	}
	if _, err = tx.ExecContext(ctx, `UPDATE requirement_index SET status=$5,error_code=$6,lease_until=clock_timestamp() WHERE workspace=$1 AND project_id=$2 AND requirement_id=$3 AND profile=$4`, workspace, project, state.RequirementID, state.Profile, state.Status, state.ErrorCode); err != nil {
		return state, errStorage
	}
	if err = tx.Commit(); err != nil {
		return state, errStorage
	}
	return state, nil
}

func (s *Service) Search(ctx context.Context, workspace, project, text string, limit int) ([]Hit, error) {
	if err := s.authorize(ctx, workspace, project); err != nil {
		return nil, err
	}
	if !utf8.ValidString(text) || utf8.RuneCountInString(text) > 2000 || strings.TrimSpace(text) == "" || limit < 1 || limit > 20 {
		return nil, ErrInvalid
	}
	profile := s.profile()
	if s.embedder == nil || profile == "" {
		return nil, ErrNotConfigured
	}
	embedCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	vectors, err := s.embedder.Embed(embedCtx, []string{text})
	if err != nil || len(vectors) != 1 {
		return nil, errors.New("search embedding unavailable")
	}
	vector, err := vectorLiteral(vectors[0])
	if err != nil {
		return nil, errors.New("invalid search embedding")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT c.requirement_id,c.project_id,c.text,r.text,c.version,c.start_offset,c.end_offset,1-(c.embedding OPERATOR(public.<=>) $4::public.vector) AS score FROM requirement_chunks c JOIN requirements r ON r.workspace=c.workspace AND r.project_id=c.project_id AND r.id=c.requirement_id AND r.version=c.version JOIN requirement_index i ON i.workspace=c.workspace AND i.project_id=c.project_id AND i.requirement_id=c.requirement_id AND i.profile=c.profile AND i.version=c.version AND i.status='ready' WHERE c.workspace=$1 AND c.project_id=$2 AND c.profile=$3 ORDER BY c.embedding OPERATOR(public.<=>) $4::public.vector,c.requirement_id,c.ordinal LIMIT $5`, workspace, project, profile, vector, limit)
	if err != nil {
		return nil, errStorage
	}
	defer rows.Close()
	out := []Hit{}
	for rows.Next() {
		var hit Hit
		if err = rows.Scan(&hit.RequirementID, &hit.ProjectID, &hit.Text, &hit.SourceText, &hit.Version, &hit.Start, &hit.End, &hit.Score); err != nil {
			return nil, errStorage
		}
		out = append(out, hit)
	}
	if rows.Err() != nil {
		return nil, errStorage
	}
	return out, nil
}

func vectorLiteral(vector []float32) (string, error) {
	if len(vector) != 1024 {
		return "", errors.New("embedding dimension mismatch")
	}
	var builder strings.Builder
	builder.WriteByte('[')
	nonzero := false
	for i, value := range vector {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return "", errors.New("embedding must be finite")
		}
		if value != 0 {
			nonzero = true
		}
		if i > 0 {
			builder.WriteByte(',')
		}
		builder.WriteString(strconv.FormatFloat(float64(value), 'g', -1, 32))
	}
	if !nonzero {
		return "", errors.New("embedding cannot be zero")
	}
	builder.WriteByte(']')
	return builder.String(), nil
}
