package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testFile = "errors.defs.yaml"

// catalogYAML returns a catalog with valid docs and the given errors, the first error is at line 5.
func catalogYAML(errors string) string {
	return "docs:\n  product: Acme\n  support_email: help@acme.test\nerrors:" + errors
}

func parseCatalog(t *testing.T, yaml string) (*catalog, error) {
	t.Helper()
	p := &problems{file: testFile}
	c := decode(testFile, []byte(yaml), p)
	if c != nil {
		validate(c, p)
	}
	return c, p.err()
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		// want are the expected problems, all of them.
		want []string
	}{
		{
			name: "valid catalog",
			yaml: catalogYAML(`
  - name: UserNotFound
    code: ERR_01001
    kind: NOT_FOUND
    common: true
    message: "user {id} not found"
    vars:
      - name: id
        type: string
      - name: tenantID
        type: int64
        internal: true
`),
		},
		{
			name: "invalid yaml",
			yaml: "docs: [",
			want: []string{"errors.defs.yaml: yaml: line 1: did not find expected node content"},
		},
		{
			name: "missing docs",
			yaml: `errors:
  - {name: A, code: ERR_00001, kind: INTERNAL, message: a}
`,
			want: []string{
				"errors.defs.yaml:1: docs: product is required",
				"errors.defs.yaml:1: docs: support_email is required",
			},
		},
		{
			name: "invalid email",
			yaml: `docs:
  product: Acme
  support_email: support
errors:
  - {name: A, code: ERR_00001, kind: INTERNAL, message: a}
`,
			want: []string{`errors.defs.yaml:3: docs: support_email "support" is not an email address`},
		},
		{
			name: "no errors",
			yaml: catalogYAML(" []\n"),
			want: []string{"errors.defs.yaml:4: errors: at least one error is required"},
		},
		{
			name: "unknown keys",
			yaml: `docs:
  product: Acme
  support_email: help@acme.test
  produc: Acme
errors:
  - name: A
    error_cod: ERR_00001
    code: ERR_00001
    kind: INTERNAL
    messag: a
    message: a
    vars:
      - name: id
        typ: string
        type: string
`,
			want: []string{
				`errors.defs.yaml:4: docs: unknown key "produc"`,
				`errors.defs.yaml:7: A: unknown key "error_cod"`,
				`errors.defs.yaml:10: A: unknown key "messag"`,
				`errors.defs.yaml:14: A: unknown key "typ"`,
			},
		},
		{
			name: "duplicate key",
			yaml: catalogYAML(`
  - name: A
    code: ERR_00001
    kind: INTERNAL
    message: a
    code: ERR_00002
`),
			want: []string{`errors.defs.yaml:9: A: duplicate key "code"`},
		},
		{
			name: "duplicate name",
			yaml: catalogYAML(`
  - {name: A, code: ERR_00001, kind: INTERNAL, message: a}
  - {name: A, code: ERR_00002, kind: INTERNAL, message: a}
`),
			want: []string{"errors.defs.yaml:6: A: duplicate name, already used at line 5"},
		},
		{
			name: "duplicate code",
			yaml: catalogYAML(`
  - {name: A, code: ERR_00001, kind: INTERNAL, message: a}
  - {name: B, code: ERR_00001, kind: INTERNAL, message: b}
`),
			want: []string{"errors.defs.yaml:6: B: duplicate code ERR_00001, already used by A"},
		},
		{
			name: "missing fields",
			yaml: catalogYAML(`
  - common: true
`),
			want: []string{
				"errors.defs.yaml:5: errors[0]: name is required",
				"errors.defs.yaml:5: errors[0]: code is required",
				"errors.defs.yaml:5: errors[0]: kind is required",
				"errors.defs.yaml:5: errors[0]: message is required",
			},
		},
		{
			name: "invalid name",
			yaml: catalogYAML(`
  - {name: userNotFound, code: ERR_00001, kind: INTERNAL, message: a}
  - {name: User-Not-Found, code: ERR_00002, kind: INTERNAL, message: a}
`),
			want: []string{
				"errors.defs.yaml:5: userNotFound: name must be an exported Go identifier (eg: UserNotFound)",
				"errors.defs.yaml:6: User-Not-Found: name must be an exported Go identifier (eg: UserNotFound)",
			},
		},
		{
			name: "keyword name",
			yaml: catalogYAML(`
  - {name: func, code: ERR_00001, kind: INTERNAL, message: a}
`),
			want: []string{"errors.defs.yaml:5: func: name is a Go keyword"},
		},
		{
			name: "bad code format",
			yaml: catalogYAML(`
  - {name: A, code: ERR_SAMPLE_00001, kind: INTERNAL, message: a}
  - {name: B, code: ERR_1234, kind: INTERNAL, message: b}
  - {name: C, code: ERR_000001, kind: INTERNAL, message: c}
  - {name: D, code: err_00001, kind: INTERNAL, message: d}
`),
			want: []string{
				`errors.defs.yaml:5: A: code "ERR_SAMPLE_00001" must match ^ERR_[0-9]{5}$ (eg: ERR_01001)`,
				`errors.defs.yaml:6: B: code "ERR_1234" must match ^ERR_[0-9]{5}$ (eg: ERR_01001)`,
				`errors.defs.yaml:7: C: code "ERR_000001" must match ^ERR_[0-9]{5}$ (eg: ERR_01001)`,
				`errors.defs.yaml:8: D: code "err_00001" must match ^ERR_[0-9]{5}$ (eg: ERR_01001)`,
			},
		},
		{
			name: "bad kind",
			yaml: catalogYAML(`
  - {name: A, code: ERR_00001, kind: Internal, message: a}
  - {name: B, code: ERR_00002, kind: CANCELED, message: b}
`),
			want: []string{
				`errors.defs.yaml:5: A: unknown kind "Internal", want a kind name (eg: NOT_FOUND)`,
				`errors.defs.yaml:6: B: unknown kind "CANCELED", want a kind name (eg: NOT_FOUND)`,
			},
		},
		{
			name: "OK kind",
			yaml: catalogYAML(`
  - {name: A, code: ERR_00001, kind: OK, message: a}
`),
			want: []string{"errors.defs.yaml:5: A: kind OK is not an error kind"},
		},
		{
			name: "old status key",
			yaml: catalogYAML(`
  - name: A
    code: ERR_00001
    status: INTERNAL
    message: a
`),
			want: []string{
				`errors.defs.yaml:7: A: unknown key "status"`,
				"errors.defs.yaml:5: A: kind is required",
			},
		},
		{
			name: "empty message",
			yaml: catalogYAML(`
  - name: A
    code: ERR_00001
    kind: INTERNAL
    message: ""
`),
			want: []string{"errors.defs.yaml:8: A: message is required"},
		},
		{
			name: "multi-line message",
			yaml: catalogYAML(`
  - name: A
    code: ERR_00001
    kind: INTERNAL
    message: "a\nb"
`),
			want: []string{"errors.defs.yaml:8: A: message must be a single line without control characters"},
		},
		{
			name: "malformed placeholders",
			yaml: catalogYAML(`
  - {name: A, code: ERR_00001, kind: INTERNAL, message: "user {id:string}"}
  - {name: B, code: ERR_00002, kind: INTERNAL, message: "user { id}"}
  - {name: C, code: ERR_00003, kind: INTERNAL, message: "user {}"}
  - {name: D, code: ERR_00004, kind: INTERNAL, message: "user {id"}
  - {name: E, code: ERR_00005, kind: INTERNAL, message: "user id}"}
  - {name: F, code: ERR_00006, kind: INTERNAL, message: "user {a{id}}"}
`),
			want: []string{
				"errors.defs.yaml:5: A: message: malformed placeholder {id:string}, want {name}",
				"errors.defs.yaml:6: B: message: malformed placeholder { id}, want {name}",
				"errors.defs.yaml:7: C: message: malformed placeholder {}, want {name}",
				`errors.defs.yaml:8: D: message: unclosed { in "user {id"`,
				`errors.defs.yaml:9: E: message: unexpected } in "user id}"`,
				`errors.defs.yaml:10: F: message: unclosed { in "user {a{id}}"`,
			},
		},
		{
			name: "unknown placeholder",
			yaml: catalogYAML(`
  - name: A
    code: ERR_00001
    kind: INTERNAL
    message: "user {userId} not found"
    vars:
      - {name: id, type: string}
`),
			want: []string{"errors.defs.yaml:8: A: message: placeholder {userId} is not a declared var"},
		},
		{
			name: "internal placeholder",
			yaml: catalogYAML(`
  - name: A
    code: ERR_00001
    kind: INTERNAL
    message: "user {id} not found"
    vars:
      - {name: id, type: string, internal: true}
`),
			want: []string{"errors.defs.yaml:8: A: message: placeholder {id} references an internal var, it would be sent to the client"},
		},
		{
			name: "bad var type",
			yaml: catalogYAML(`
  - name: A
    code: ERR_00001
    kind: INTERNAL
    message: a
    vars:
      - name: id
        type: uuid
      - name: count
`),
			want: []string{
				`errors.defs.yaml:11: A: var id: unsupported type "uuid", want one of string, bool, int, int32, int64, uint, uint32, uint64, float32, float64`,
				"errors.defs.yaml:12: A: var count: type is required",
			},
		},
		{
			name: "invalid var names",
			yaml: catalogYAML(`
  - name: A
    code: ERR_00001
    kind: INTERNAL
    message: a
    vars:
      - {name: id, type: string}
      - {name: id, type: string}
      - {name: type, type: string}
      - {name: ID, type: string}
      - {name: fmt, type: string}
      - {name: any, type: string}
      - {name: userID, type: string}
      - {name: userId, type: string}
      - {type: string}
      - {name: internal, type: string, internal: maybe}
`),
			want: []string{
				"errors.defs.yaml:11: A: duplicate var id",
				"errors.defs.yaml:12: A: var type is a Go keyword",
				`errors.defs.yaml:13: A: var "ID" must be a lowerCamel Go identifier`,
				"errors.defs.yaml:14: A: var fmt shadows a Go identifier used by the generated code",
				"errors.defs.yaml:15: A: var any shadows a Go identifier used by the generated code",
				`errors.defs.yaml:17: A: var userId has the same detail key "user_id" as var userID`,
				"errors.defs.yaml:18: A: var name is required",
				"errors.defs.yaml:19: A: var internal must be a boolean",
			},
		},
		{
			name: "Code suffix collision",
			yaml: catalogYAML(`
  - {name: User, code: ERR_00001, kind: INTERNAL, message: a}
  - {name: UserCode, code: ERR_00002, kind: INTERNAL, message: a}
`),
			want: []string{"errors.defs.yaml:6: UserCode: name collides with the generated constant UserCode of User"},
		},
		{
			name: "wrong node kinds",
			yaml: catalogYAML(`
  - name: A
    code: [ERR_00001]
    kind: INTERNAL
    message: a
    vars: id
`),
			want: []string{
				"errors.defs.yaml:6: A: code must be a string",
				"errors.defs.yaml:6: A: code is required",
				"errors.defs.yaml:9: A: vars must be a list",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseCatalog(t, tt.yaml)
			if len(tt.want) == 0 {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.ElementsMatch(t, tt.want, splitLines(err.Error()))
		})
	}
}

func TestParseMessage(t *testing.T) {
	segments, err := parseMessage("user {id} has {count}%")
	require.NoError(t, err)
	assert.Equal(t, []segment{
		{text: "user "},
		{text: "id", placeholder: true},
		{text: " has "},
		{text: "count", placeholder: true},
		{text: "%"},
	}, segments)
}
