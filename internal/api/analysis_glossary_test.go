package api

import (
	"reflect"
	"testing"
)

func TestSavedAnalysisExplanationGlossary(t *testing.T) {
	s, err := New(testDatabase(t), map[string]string{"token-a": "a", "token-b": "b"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, err = s.db.Exec(`
		INSERT INTO projects VALUES ('p','a','P','G','','now','now'),('q','a','Q','G','','now','now'),('f','b','F','G','','now','now');
		INSERT INTO glossary_terms(id,workspace,project_id,name,variants,definition,meaning_context,version,source_id,source_version) VALUES
			('one','a','p','Alpha Beta','[{"text":"АБ","kind":"abbreviation"}]','Definition','',1,'',0),
			('two','a','p','Another Business','[{"text":"АБ","kind":"abbreviation"}]','Definition','',1,'',0),
			('foreign-project','a','q','АБ','[]','Definition','',1,'',0),('foreign-workspace','b','f','АБ','[]','Definition','',1,'',0);
		INSERT INTO analysis_runs(id,workspace,project_id,status,outcome,generator_profile,embedding_profile,source_signature,sources,lease_until)
			VALUES ('run','a','p','completed','no_collisions_found','g','e','sig','[]',clock_timestamp());
		INSERT INTO analysis_pairs(id,run_id,workspace,project_id,position,left_source,right_source,generator_profile,cache_key,status,verdict,explanation,result_origin) VALUES
			('saved','run','a','p',1,'{"id":"a"}','{"id":"b"}','g','k1','completed','collision','😀 аб, ХАБ',''),
			('exact','run','a','p',2,'{"id":"a"}','{"id":"c"}','g','k2','completed','duplicate','АБ','exact_text_v1'),
			('unfinished','run','a','p',3,'{"id":"b"}','{"id":"c"}','g','k3','running','','АБ','model');
	`)
	if err != nil {
		t.Fatal(err)
	}

	const detail = `analysisPair(projectId:"p",analysisId:"run",pairId:"saved")`
	const selection = `{explanation annotated:explanationMatches{start end text terms{id}}}`
	read := func(q string) map[string]any {
		t.Helper()
		out := summaryRequest(t, s, "token-a", q)
		if out["errors"] != nil {
			t.Fatal(out)
		}
		return out["data"].(map[string]any)
	}
	want := map[string]any{"explanation": "😀 аб, ХАБ", "annotated": []any{map[string]any{
		"start": float64(2), "end": float64(4), "text": "аб", "terms": []any{map[string]any{"id": "one"}, map[string]any{"id": "two"}},
	}}}
	if got := read(`{` + detail + selection + `}`)["analysisPair"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("saved explanation and scoped matches: got %v, want %v", got, want)
	}
	if other := summaryRequest(t, s, "token-b", `{`+detail+selection+`}`); other["errors"] == nil || other["data"].(map[string]any)["analysisPair"] != nil {
		t.Fatalf("foreign pair visible: %v", other)
	}
	pairs := read(`{analysisPairs(projectId:"p",analysisId:"run",limit:10,offset:0,status:"",verdict:"")` + selection + `}`)["analysisPairs"].([]any)
	if !reflect.DeepEqual(pairs, []any{want, map[string]any{"explanation": "АБ", "annotated": []any{}}, map[string]any{"explanation": "АБ", "annotated": []any{}}}) {
		t.Fatalf("unexpected pair annotations: %v", pairs)
	}
	run := read(`{projectAnalysis(projectId:"p",analysisId:"run"){pairs` + selection + `}}`)["projectAnalysis"].(map[string]any)
	if !reflect.DeepEqual(run["pairs"], pairs) {
		t.Fatalf("nested run pairs: %v", run)
	}
	conn := read(`{analysisPairsConnection(projectId:"p",analysisId:"run",first:1){edges{node` + selection + `}}}`)["analysisPairsConnection"].(map[string]any)
	if !reflect.DeepEqual(conn["edges"].([]any)[0].(map[string]any)["node"], want) {
		t.Fatalf("connection pair annotations: %v", conn)
	}

	// An operation reading only queue or provenance data must not load the glossary.
	if _, err := s.db.Exec(`ALTER TABLE glossary_terms RENAME TO unavailable_terms`); err != nil {
		t.Fatal(err)
	}
	read(`{` + detail + `{generatorProfile}}`)
	read(`{analysisPairsConnection(projectId:"p",analysisId:"run",first:1){edges{node{id}}}}`)
}
