package api

import (
	"errors"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"
)

// validateQueryBudget limits expanded fields before any resolver can execute.
// All operations are counted conservatively, even if operationName selects one.
func validateQueryBudget(query string) error {
	doc, err := parser.ParseQueryWithTokenLimit(&ast.Source{Input: query}, 10000)
	if err != nil {
		return err
	}
	remaining := 100
	active := map[string]bool{}
	var walk func(ast.SelectionSet, int) error
	walk = func(set ast.SelectionSet, depth int) error {
		if depth > 32 {
			return errors.New("query nesting limit exceeded")
		}
		for _, entry := range set {
			remaining--
			if remaining < 0 {
				return errors.New("query field limit exceeded")
			}
			switch v := entry.(type) {
			case *ast.Field:
				if err := walk(v.SelectionSet, depth+1); err != nil {
					return err
				}
			case *ast.InlineFragment:
				if err := walk(v.SelectionSet, depth+1); err != nil {
					return err
				}
			case *ast.FragmentSpread:
				frag := doc.Fragments.ForName(v.Name)
				if frag == nil || active[v.Name] {
					return errors.New("invalid fragment")
				}
				active[v.Name] = true
				err := walk(frag.SelectionSet, depth+1)
				delete(active, v.Name)
				if err != nil {
					return err
				}
			default:
				return errors.New("unsupported selection")
			}
		}
		return nil
	}
	for _, op := range doc.Operations {
		if err := walk(op.SelectionSet, 0); err != nil {
			return err
		}
	}
	return nil
}
