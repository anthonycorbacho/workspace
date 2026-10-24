package main

import (
	"errors"
	"fmt"
	"go/token"
	"go/types"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

var codePattern = regexp.MustCompile(`^ERR_[0-9]{5}$`)

var varTypes = []string{"string", "bool", "int", "int32", "int64", "uint", "uint32", "uint64", "float32", "float64"}

// problem is a catalog error located in the YAML file.
type problem struct {
	line    int
	subject string
	msg     string
}

// problems collects every catalog error, so that they can all be fixed at once.
type problems struct {
	file string
	list []problem
}

func (p *problems) add(line int, subject, format string, args ...any) {
	p.list = append(p.list, problem{line: line, subject: subject, msg: fmt.Sprintf(format, args...)})
}

// err returns the problems as a single error, one problem per line ordered by line, or nil.
func (p *problems) err() error {
	if len(p.list) == 0 {
		return nil
	}
	slices.SortStableFunc(p.list, func(a, b problem) int { return a.line - b.line })
	var b strings.Builder
	for i, pb := range p.list {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(p.file)
		if pb.line > 0 {
			fmt.Fprintf(&b, ":%d", pb.line)
		}
		b.WriteString(": ")
		if pb.subject != "" {
			b.WriteString(pb.subject + ": ")
		}
		b.WriteString(pb.msg)
	}
	return errors.New(b.String())
}

// validate reports every problem of the catalog in p and parses the messages of the errors.
func validate(c *catalog, p *problems) {
	validateDocs(c, p)
	if len(c.Errors) == 0 {
		p.add(lineOf(c.Lines, "errors", c.Line), "errors", "at least one error is required")
	}

	names := map[string]*errorDef{}
	codes := map[string]*errorDef{}
	for i := range c.Errors {
		e := &c.Errors[i]
		validateError(e, p)
		if other, ok := names[e.Name]; ok && e.Name != "" {
			p.add(e.line("name"), e.subject(), "duplicate name, already used at line %d", other.line("name"))
		} else {
			names[e.Name] = e
		}
		if other, ok := codes[e.Code]; ok && e.Code != "" {
			p.add(e.line("code"), e.subject(), "duplicate code %s, already used by %s", e.Code, other.subject())
		} else {
			codes[e.Code] = e
		}
	}

	// the generated <Name>Code constant lives in the same package as the constructors.
	for _, e := range c.Errors {
		if other, ok := names[e.Name+"Code"]; ok && e.Name != "" {
			p.add(other.line("name"), other.subject(), "name collides with the generated constant %sCode of %s", e.Name, e.Name)
		}
	}
}

func validateDocs(c *catalog, p *problems) {
	d := c.Docs
	line := func(key string) int { return lineOf(d.Lines, key, lineOf(c.Lines, "docs", c.Line)) }
	if d.Product == "" {
		p.add(line("product"), "docs", "product is required")
	}
	switch {
	case d.SupportEmail == "":
		p.add(line("support_email"), "docs", "support_email is required")
	case !strings.Contains(d.SupportEmail, "@"):
		p.add(line("support_email"), "docs", "support_email %q is not an email address", d.SupportEmail)
	}
}

func validateError(e *errorDef, p *problems) {
	switch {
	case e.Name == "":
		p.add(e.line("name"), e.subject(), "name is required")
	case token.IsKeyword(e.Name):
		p.add(e.line("name"), e.subject(), "name is a Go keyword")
	case !token.IsIdentifier(e.Name) || !token.IsExported(e.Name):
		p.add(e.line("name"), e.subject(), "name must be an exported Go identifier (eg: UserNotFound)")
	}

	switch {
	case e.Code == "":
		p.add(e.line("code"), e.subject(), "code is required")
	case !codePattern.MatchString(e.Code):
		p.add(e.line("code"), e.subject(), "code %q must match %s (eg: ERR_01001)", e.Code, codePattern)
	}

	switch _, ok := kinds[e.Kind]; {
	case e.Kind == "":
		p.add(e.line("kind"), e.subject(), "kind is required")
	case e.Kind == "OK":
		p.add(e.line("kind"), e.subject(), "kind OK is not an error kind")
	case !ok:
		p.add(e.line("kind"), e.subject(), "unknown kind %q, want a kind name (eg: NOT_FOUND)", e.Kind)
	}

	validateVars(e, p)
	validateMessage(e, p)
}

func validateVars(e *errorDef, p *problems) {
	names := map[string]bool{}
	keys := map[string]string{}
	for _, v := range e.Vars {
		line := func(key string) int { return lineOf(v.Lines, key, v.Line) }
		switch {
		case v.Name == "":
			p.add(line("name"), e.subject(), "var name is required")
		case token.IsKeyword(v.Name):
			p.add(line("name"), e.subject(), "var %s is a Go keyword", v.Name)
		case !token.IsIdentifier(v.Name) || token.IsExported(v.Name) || v.Name == "_":
			p.add(line("name"), e.subject(), "var %q must be a lowerCamel Go identifier", v.Name)
		case v.Name == "fmt" || types.Universe.Lookup(v.Name) != nil:
			// the var is a constructor parameter, it would shadow the identifier in the generated code.
			p.add(line("name"), e.subject(), "var %s shadows a Go identifier used by the generated code", v.Name)
		case names[v.Name]:
			p.add(line("name"), e.subject(), "duplicate var %s", v.Name)
		case keys[snakeCase(v.Name)] != "":
			p.add(line("name"), e.subject(), "var %s has the same detail key %q as var %s", v.Name, snakeCase(v.Name), keys[snakeCase(v.Name)])
		}
		names[v.Name] = true
		if _, ok := keys[snakeCase(v.Name)]; !ok {
			keys[snakeCase(v.Name)] = v.Name
		}

		switch {
		case v.Type == "":
			p.add(line("type"), e.subject(), "var %s: type is required", v.Name)
		case !slices.Contains(varTypes, v.Type):
			p.add(line("type"), e.subject(), "var %s: unsupported type %q, want one of %s", v.Name, v.Type, strings.Join(varTypes, ", "))
		}
	}
}

func validateMessage(e *errorDef, p *problems) {
	line := e.line("message")
	if e.Message == "" {
		p.add(line, e.subject(), "message is required")
		return
	}
	// the message is copied in a // comment of the generated code.
	if strings.ContainsFunc(e.Message, unicode.IsControl) {
		p.add(line, e.subject(), "message must be a single line without control characters")
		return
	}
	segments, err := parseMessage(e.Message)
	if err != nil {
		p.add(line, e.subject(), "message: %v", err)
		return
	}
	e.Segments = segments

	for _, s := range segments {
		if !s.placeholder {
			continue
		}
		i := slices.IndexFunc(e.Vars, func(v varDef) bool { return v.Name == s.text })
		switch {
		case i < 0:
			p.add(line, e.subject(), "message: placeholder {%s} is not a declared var", s.text)
		case e.Vars[i].Internal:
			p.add(line, e.subject(), "message: placeholder {%s} references an internal var, it would be sent to the client", s.text)
		}
	}
}

// segment is a part of a message: either literal text or a {name} placeholder.
type segment struct {
	text        string
	placeholder bool
}

// parseMessage splits a message into literal text and {name} placeholders.
// Braces are reserved for placeholders, any other use of them is an error.
func parseMessage(msg string) ([]segment, error) {
	var segments []segment
	rest := msg
	for rest != "" {
		open := strings.IndexAny(rest, "{}")
		if open < 0 {
			segments = append(segments, segment{text: rest})
			break
		}
		if rest[open] == '}' {
			return nil, fmt.Errorf("unexpected } in %q", msg)
		}
		if open > 0 {
			segments = append(segments, segment{text: rest[:open]})
		}
		rest = rest[open+1:]
		end := strings.IndexAny(rest, "{}")
		if end < 0 || rest[end] == '{' {
			return nil, fmt.Errorf("unclosed { in %q", msg)
		}
		name := rest[:end]
		if !token.IsIdentifier(name) {
			return nil, fmt.Errorf("malformed placeholder {%s}, want {name}", name)
		}
		segments = append(segments, segment{text: name, placeholder: true})
		rest = rest[end+1:]
	}
	return segments, nil
}

// snakeCase converts a lowerCamel var name to the snake_case key of the error details, eg: userID -> user_id.
func snakeCase(s string) string {
	runes := []rune(s)
	var b strings.Builder
	for i, r := range runes {
		if unicode.IsUpper(r) && i > 0 {
			prev := runes[i-1]
			endOfAcronym := unicode.IsUpper(prev) && i+1 < len(runes) && unicode.IsLower(runes[i+1])
			if unicode.IsLower(prev) || unicode.IsDigit(prev) || endOfAcronym {
				b.WriteByte('_')
			}
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}
