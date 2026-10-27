package api

import (
	"fmt"
	"strings"
	"testing"
)

func TestQueryBudgetFragments(t *testing.T) {
	if err := validateQueryBudget(`{ projects(limit:1,offset:0){...F} } fragment F on Project {name}`); err != nil {
		t.Fatal(err)
	}
	if err := validateQueryBudget(`{ projects(limit:1,offset:0){...F} } fragment F on Project {...F}`); err == nil {
		t.Fatal("recursive fragment accepted")
	}
	var q strings.Builder
	q.WriteString(`{ projects(limit:1,offset:0){`)
	for i := 0; i < 101; i++ {
		fmt.Fprint(&q, "...F ")
	}
	q.WriteString(`} } fragment F on Project { name }`)
	if err := validateQueryBudget(q.String()); err == nil {
		t.Fatal("expanded fragments accepted")
	}
}
