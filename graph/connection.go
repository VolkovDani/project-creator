package graph

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
)

const defaultConnectionSize = 40
const maxConnectionSize = 100

type connectionCursor struct {
	Version int    `json:"v"`
	Kind    string `json:"k"`
	Scope   string `json:"s"`
	Key     string `json:"x"`
	ID      string `json:"i"`
}

func encodeConnectionCursor(kind, scope, key, id string) string {
	raw, _ := json.Marshal(connectionCursor{Version: 1, Kind: kind, Scope: scope, Key: key, ID: id})
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeConnectionCursor(value, kind, scope string) (connectionCursor, error) {
	var cursor connectionCursor
	if len(value) == 0 || len(value) > 8192 {
		return connectionCursor{}, errors.New("invalid pagination cursor")
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || json.Unmarshal(raw, &cursor) != nil || cursor.Version != 1 || cursor.Kind != kind || cursor.Scope != scope || cursor.Key == "" || cursor.ID == "" {
		return connectionCursor{}, errors.New("invalid pagination cursor")
	}
	return cursor, nil
}

func connectionSize(first *int32) (int, error) {
	if first == nil {
		return defaultConnectionSize, nil
	}
	if *first < 1 || *first > maxConnectionSize {
		return 0, errors.New("invalid pagination")
	}
	return int(*first), nil
}

func (r *Resolver) ProjectsConnection(ctx context.Context, args ProjectsConnectionArgs) (*ProjectConnection, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	first, err := connectionSize(args.First)
	if err != nil {
		return nil, err
	}
	curScope := workspace(ctx)
	var key, id string
	if args.After != nil {
		cursor, e := decodeConnectionCursor(*args.After, "projects", curScope)
		if e != nil {
			return nil, e
		}
		key, id = cursor.Key, cursor.ID
	}
	tx, err := r.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, errors.New("storage failure")
	}
	defer tx.Rollback()
	var total int32
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM projects WHERE workspace=$1`, curScope).Scan(&total); err != nil {
		return nil, errors.New("storage failure")
	}
	query := `SELECT id,name,goal,description,created_at,updated_at FROM projects WHERE workspace=$1`
	var rows *sql.Rows
	if args.After == nil {
		rows, err = tx.QueryContext(ctx, query+` ORDER BY created_at,id LIMIT $2`, curScope, first+1)
	} else {
		rows, err = tx.QueryContext(ctx, query+` AND (created_at,id)>($2,$3) ORDER BY created_at,id LIMIT $4`, curScope, key, id, first+1)
	}
	if err != nil {
		return nil, errors.New("storage failure")
	}
	defer rows.Close()
	items := []*Project{}
	for rows.Next() {
		p := &Project{}
		if err = rows.Scan(&p.ID, &p.Name, &p.Goal, &p.Description, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, errors.New("storage failure")
		}
		items = append(items, p)
	}
	if rows.Err() != nil {
		return nil, errors.New("storage failure")
	}
	hasNext := len(items) > first
	if hasNext {
		items = items[:first]
	}
	conn := &ProjectConnection{Edges: []*ProjectEdge{}, PageInfo: &PageInfo{HasNextPage: hasNext}, TotalCount: total}
	for _, p := range items {
		c := encodeConnectionCursor("projects", curScope, p.CreatedAt, string(p.ID))
		conn.Edges = append(conn.Edges, &ProjectEdge{Cursor: c, Node: p})
		conn.PageInfo.EndCursor = &c
	}
	if err = tx.Commit(); err != nil {
		return nil, errors.New("storage failure")
	}
	return conn, nil
}

func (r *Resolver) RequirementsConnection(ctx context.Context, args RequirementsConnectionArgs) (*RequirementConnection, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	first, err := connectionSize(args.First)
	if err != nil {
		return nil, err
	}
	scope := workspace(ctx) + "\x00" + string(args.ProjectId)
	var key, id string
	if args.After != nil {
		c, e := decodeConnectionCursor(*args.After, "requirements", scope)
		if e != nil {
			return nil, e
		}
		key, id = c.Key, c.ID
	}
	tx, err := r.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, errors.New("storage failure")
	}
	defer tx.Rollback()
	var total int32
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM requirements WHERE workspace=$1 AND project_id=$2`, workspace(ctx), args.ProjectId).Scan(&total); err != nil {
		return nil, errors.New("storage failure")
	}
	query := `SELECT id,project_id,text,version,created_at,updated_at FROM requirements WHERE workspace=$1 AND project_id=$2`
	var rows *sql.Rows
	if args.After == nil {
		rows, err = tx.QueryContext(ctx, query+` ORDER BY created_at,id LIMIT $3`, workspace(ctx), args.ProjectId, first+1)
	} else {
		rows, err = tx.QueryContext(ctx, query+` AND (created_at,id)>($3,$4) ORDER BY created_at,id LIMIT $5`, workspace(ctx), args.ProjectId, key, id, first+1)
	}
	if err != nil {
		return nil, errors.New("storage failure")
	}
	defer rows.Close()
	items := []*Requirement{}
	for rows.Next() {
		p := &Requirement{}
		if err = rows.Scan(&p.ID, &p.ProjectId, &p.Text, &p.Version, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, errors.New("storage failure")
		}
		items = append(items, p)
	}
	if rows.Err() != nil {
		return nil, errors.New("storage failure")
	}
	hasNext := len(items) > first
	if hasNext {
		items = items[:first]
	}
	conn := &RequirementConnection{Edges: []*RequirementEdge{}, PageInfo: &PageInfo{HasNextPage: hasNext}, TotalCount: total}
	for _, p := range items {
		c := encodeConnectionCursor("requirements", scope, p.CreatedAt, string(p.ID))
		conn.Edges = append(conn.Edges, &RequirementEdge{Cursor: c, Node: p})
		conn.PageInfo.EndCursor = &c
	}
	if err = tx.Commit(); err != nil {
		return nil, errors.New("storage failure")
	}
	return conn, nil
}

func (r *Resolver) AnalysisPairsConnection(ctx context.Context, args AnalysisPairsConnectionArgs) (*AnalysisPairConnection, error) {
	if err := consume(ctx); err != nil {
		return nil, err
	}
	first, err := connectionSize(args.First)
	if err != nil {
		return nil, err
	}
	status, verdict := "", ""
	if args.Status != nil {
		status = *args.Status
	}
	if args.Verdict != nil {
		verdict = *args.Verdict
	}
	scope := workspace(ctx) + "\x00" + string(args.ProjectId) + "\x00" + string(args.AnalysisId) + "\x00" + status + "\x00" + verdict
	var pos int64
	var id string
	if args.After != nil {
		c, e := decodeConnectionCursor(*args.After, "analysisPairs", scope)
		if e != nil {
			return nil, e
		}
		if pos, err = strconv.ParseInt(c.Key, 10, 64); err != nil {
			return nil, errors.New("invalid pagination cursor")
		}
		id = c.ID
	}
	pairs, total, hasNext, err := r.Analysis.PairsConnection(ctx, workspace(ctx), string(args.ProjectId), string(args.AnalysisId), first, status, verdict, args.After != nil, pos, id)
	if err != nil {
		return nil, errors.New("analysis unavailable")
	}
	conn := &AnalysisPairConnection{Edges: []*AnalysisPairEdge{}, PageInfo: &PageInfo{HasNextPage: hasNext}, TotalCount: int32(total)}
	nodes := make([]*AnalysisPair, 0, len(pairs))
	for _, pair := range pairs {
		c := encodeConnectionCursor("analysisPairs", scope, strconv.FormatInt(int64(pair.Position), 10), pair.ID)
		node := analysisPair(pair)
		nodes = append(nodes, node)
		conn.Edges = append(conn.Edges, &AnalysisPairEdge{Cursor: c, Node: node})
		conn.PageInfo.EndCursor = &c
	}
	return conn, r.matchPairExplanations(ctx, string(args.ProjectId), "edges.node.explanationMatches", nodes)
}
