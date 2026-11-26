package codegen

import (
	"fmt"
	"go/format"
	"regexp"
	"strings"
	"unicode"
)

var token = regexp.MustCompile(`^[A-Za-z_][A-Za-z_0-9]*|^[!\[\]{}():]`)

type parser struct {
	t      []string
	i      int
	names  map[string]bool
	inputs map[string]bool
}

func (p *parser) pop() (string, error) {
	if p.i >= len(p.t) {
		return "", fmt.Errorf("unexpected end")
	}
	s := p.t[p.i]
	p.i++
	return s, nil
}
func (p *parser) want(s string) error {
	v, e := p.pop()
	if e != nil || v != s {
		return fmt.Errorf("expected %s, got %s", s, v)
	}
	return nil
}
func (p *parser) peek(s string) bool { return p.i < len(p.t) && p.t[p.i] == s }
func ident(s string) string {
	if s == "id" {
		return "ID"
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
func (p *parser) typ() (string, error) {
	if p.peek("[") {
		p.i++
		x, e := p.typ()
		if e != nil {
			return "", e
		}
		if e = p.want("]"); e != nil {
			return "", e
		}
		if !p.peek("!") {
			return "", fmt.Errorf("nullable lists unsupported")
		}
		p.i++
		return "[]" + x, nil
	}
	n, e := p.pop()
	if e != nil {
		return "", e
	}
	var t string
	switch n {
	case "String":
		t = "string"
	case "Boolean":
		t = "bool"
	case "Int":
		t = "int32"
	case "Float":
		t = "float64"
	case "ID":
		t = "graphql.ID"
	default:
		if !p.names[n] {
			return "", fmt.Errorf("unsupported type %s", n)
		}
		if p.inputs[n] {
			t = n
		} else {
			t = "*" + n
		}
	}
	if p.peek("!") {
		p.i++
		return t, nil
	}
	if strings.HasPrefix(t, "*") {
		return t, nil
	}
	return "*" + t, nil
}
func Generate(src string) ([]byte, error) {
	var toks []string
	for len(src) > 0 {
		if unicode.IsSpace(rune(src[0])) {
			src = src[1:]
			continue
		}
		m := token.FindString(src)
		if m == "" {
			return nil, fmt.Errorf("unsupported SDL near %q", src)
		}
		toks = append(toks, m)
		src = src[len(m):]
	}
	p := parser{t: toks, names: map[string]bool{}, inputs: map[string]bool{}}
	for i := 0; i+1 < len(toks); i++ {
		if toks[i] == "type" || toks[i] == "input" || toks[i] == "enum" {
			p.names[toks[i+1]] = true
			if toks[i] != "type" {
				p.inputs[toks[i+1]] = true
			}
		}
	}
	var b strings.Builder
	b.WriteString("// Code generated from schema.graphqls; DO NOT EDIT.\npackage graph\nimport graphql \"github.com/graph-gophers/graphql-go\"\n")
	for p.i < len(p.t) {
		kind, _ := p.pop()
		if kind != "type" && kind != "input" && kind != "enum" {
			return nil, fmt.Errorf("unsupported definition %s", kind)
		}
		name, e := p.pop()
		if e != nil {
			return nil, e
		}
		if e = p.want("{"); e != nil {
			return nil, e
		}
		if kind == "enum" {
			for !p.peek("}") {
				if _, err := p.pop(); err != nil {
					return nil, err
				}
			}
			p.i++
			continue
		}
		root := name == "Query" || name == "Mutation"
		if !root {
			fmt.Fprintf(&b, "type %s struct {\n", name)
		}
		for !p.peek("}") {
			field, e := p.pop()
			if e != nil {
				return nil, e
			}
			if p.peek("(") {
				if !root {
					return nil, fmt.Errorf("field arguments unsupported")
				}
				p.i++
				fmt.Fprintf(&b, "type %sArgs struct {\n", ident(field))
				for !p.peek(")") {
					arg, e := p.pop()
					if e != nil {
						return nil, e
					}
					if e = p.want(":"); e != nil {
						return nil, e
					}
					typ, e := p.typ()
					if e != nil {
						return nil, e
					}
					fmt.Fprintf(&b, "%s %s\n", ident(arg), typ)
				}
				p.i++
				b.WriteString("}\n")
			}
			if e = p.want(":"); e != nil {
				return nil, e
			}
			typ, e := p.typ()
			if e != nil {
				return nil, e
			}
			if !root {
				fmt.Fprintf(&b, "%s %s\n", ident(field), typ)
			}
		}
		p.i++
		if !root {
			b.WriteString("}\n")
		}
	}
	return format.Source([]byte(b.String()))
}
