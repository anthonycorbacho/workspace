package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// catalog is the decoded YAML error catalog.
type catalog struct {
	File   string
	Line   int
	Docs   docs
	Errors []errorDef
	// Lines locates the top level keys, to report problems at the right place.
	Lines map[string]int
}

type docs struct {
	Product      string
	SupportEmail string
	Lines        map[string]int
}

type errorDef struct {
	Index   int
	Name    string
	Code    string
	Kind    string
	Common  bool
	Message string
	Vars    []varDef
	Line    int
	Lines   map[string]int
	// Segments is the parsed message, set by validate.
	Segments []segment
}

type varDef struct {
	Name     string
	Type     string
	Internal bool
	Line     int
	Lines    map[string]int
}

// kinds maps the kind names accepted in the catalog to their kit/errors.Kind constant,
// the names are the kit/errors.Kind names (Kind.String). OK is deliberately absent: an error can't be a success.
var kinds = map[string]string{
	"UNKNOWN":             "KindUnknown",
	"INVALID_ARGUMENT":    "KindInvalidArgument",
	"NOT_FOUND":           "KindNotFound",
	"ALREADY_EXISTS":      "KindAlreadyExists",
	"FAILED_PRECONDITION": "KindFailedPrecondition",
	"ABORTED":             "KindAborted",
	"OUT_OF_RANGE":        "KindOutOfRange",
	"UNAUTHENTICATED":     "KindUnauthenticated",
	"PERMISSION_DENIED":   "KindPermissionDenied",
	"RESOURCE_EXHAUSTED":  "KindResourceExhausted",
	"CANCELLED":           "KindCanceled",
	"DEADLINE_EXCEEDED":   "KindDeadlineExceeded",
	"UNIMPLEMENTED":       "KindUnimplemented",
	"UNAVAILABLE":         "KindUnavailable",
	"INTERNAL":            "KindInternal",
	"DATA_LOSS":           "KindDataLoss",
}

// subject names the error in problem reports, the index is the only option without a name.
func (e *errorDef) subject() string {
	if e.Name != "" {
		return e.Name
	}
	return fmt.Sprintf("errors[%d]", e.Index)
}

// line returns the line of the value of key, or of the error itself when the key is missing.
func (e *errorDef) line(key string) int {
	return lineOf(e.Lines, key, e.Line)
}

func lineOf(lines map[string]int, key string, fallback int) int {
	if l, ok := lines[key]; ok {
		return l
	}
	return fallback
}

// load reads, decodes and validates the catalog at path.
func load(path string) (*catalog, error) {
	c, p, err := decodeFile(path)
	if err != nil {
		return nil, err
	}
	validate(c, p)
	if err := p.err(); err != nil {
		return nil, err
	}
	return c, nil
}

// decodeFile reads and decodes the catalog at path without validating it.
// Decoding problems are returned in p, so the caller can add validation problems and report all at once.
func decodeFile(path string) (*catalog, *problems, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	p := &problems{file: path}
	c := decode(path, data, p)
	if c == nil {
		return nil, nil, p.err()
	}
	return c, p, nil
}

// decode decodes the catalog from a yaml.Node tree rather than a struct, to know the line of each value
// and to reject unknown and duplicate keys. It returns nil if the YAML can't be parsed at all.
func decode(file string, data []byte, p *problems) *catalog {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		p.add(0, "", "%v", err)
		return nil
	}
	if len(root.Content) == 0 {
		p.add(0, "", "the catalog is empty")
		return nil
	}

	d := decoder{p: p}
	c := &catalog{File: file, Line: root.Content[0].Line}
	c.Lines = d.fields(root.Content[0], "catalog", func(key string, v *yaml.Node) bool {
		switch key {
		case "docs":
			c.Docs = d.docs(v)
		case "errors":
			c.Errors = d.errors(v)
		default:
			return false
		}
		return true
	})
	return c
}

type decoder struct {
	p *problems
}

// fields calls set for each key of the mapping n, reports duplicate and unknown keys (set returns false)
// and returns the line of each value.
func (d decoder) fields(n *yaml.Node, subject string, set func(key string, v *yaml.Node) bool) map[string]int {
	lines := map[string]int{}
	if n.Kind != yaml.MappingNode {
		d.p.add(n.Line, subject, "must be a mapping")
		return lines
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		if _, ok := lines[k.Value]; ok {
			d.p.add(k.Line, subject, "duplicate key %q", k.Value)
			continue
		}
		lines[k.Value] = v.Line
		if !set(k.Value, v) {
			d.p.add(k.Line, subject, "unknown key %q", k.Value)
		}
	}
	return lines
}

func (d decoder) docs(n *yaml.Node) docs {
	var doc docs
	doc.Lines = d.fields(n, "docs", func(key string, v *yaml.Node) bool {
		switch key {
		case "product":
			doc.Product = d.str(v, "docs", key)
		case "support_email":
			doc.SupportEmail = d.str(v, "docs", key)
		default:
			return false
		}
		return true
	})
	return doc
}

func (d decoder) errors(n *yaml.Node) []errorDef {
	if n.Kind != yaml.SequenceNode {
		d.p.add(n.Line, "errors", "must be a list")
		return nil
	}
	defs := make([]errorDef, 0, len(n.Content))
	for i, item := range n.Content {
		defs = append(defs, d.errorDef(i, item))
	}
	return defs
}

func (d decoder) errorDef(index int, n *yaml.Node) errorDef {
	// the name is looked up first so that every problem of the error, even before
	// the name key, is reported with the error name.
	e := errorDef{Index: index, Line: n.Line, Name: scalar(n, "name")}
	subject := e.subject()
	e.Lines = d.fields(n, subject, func(key string, v *yaml.Node) bool {
		switch key {
		case "name":
			e.Name = d.str(v, subject, key)
		case "code":
			e.Code = d.str(v, subject, key)
		case "kind":
			e.Kind = d.str(v, subject, key)
		case "common":
			e.Common = d.boolean(v, subject, key)
		case "message":
			e.Message = d.str(v, subject, key)
		case "vars":
			e.Vars = d.vars(v, subject)
		default:
			return false
		}
		return true
	})
	return e
}

func (d decoder) vars(n *yaml.Node, subject string) []varDef {
	if n.Kind != yaml.SequenceNode {
		d.p.add(n.Line, subject, "vars must be a list")
		return nil
	}
	vars := make([]varDef, 0, len(n.Content))
	for _, item := range n.Content {
		v := varDef{Line: item.Line}
		v.Lines = d.fields(item, subject, func(key string, val *yaml.Node) bool {
			switch key {
			case "name":
				v.Name = d.str(val, subject, "var name")
			case "type":
				v.Type = d.str(val, subject, "var type")
			case "internal":
				v.Internal = d.boolean(val, subject, "var internal")
			default:
				return false
			}
			return true
		})
		vars = append(vars, v)
	}
	return vars
}

func (d decoder) str(v *yaml.Node, subject, key string) string {
	if v.Kind != yaml.ScalarNode {
		d.p.add(v.Line, subject, "%s must be a string", key)
		return ""
	}
	if v.Tag == "!!null" {
		return ""
	}
	return v.Value
}

func (d decoder) boolean(v *yaml.Node, subject, key string) bool {
	var b bool
	if v.Kind != yaml.ScalarNode || v.Decode(&b) != nil {
		d.p.add(v.Line, subject, "%s must be a boolean", key)
	}
	return b
}

// scalar returns the value of the scalar key of the mapping n, or "" if there is none.
func scalar(n *yaml.Node, key string) string {
	if n.Kind != yaml.MappingNode {
		return ""
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key && n.Content[i+1].Kind == yaml.ScalarNode {
			return n.Content[i+1].Value
		}
	}
	return ""
}
