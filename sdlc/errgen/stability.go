package main

// checkStability reports the changes from the base catalog that break the API: error codes are
// documented and used by clients, so a released code must never change nor be reused by another error.
// Removing an error is fine as long as its code is not reused.
func checkStability(base, current *catalog) error {
	p := &problems{file: current.File}
	byName := map[string]errorDef{}
	for _, e := range current.Errors {
		byName[e.Name] = e
	}
	baseByCode := map[string]errorDef{}
	for _, b := range base.Errors {
		baseByCode[b.Code] = b
		if e, ok := byName[b.Name]; ok && e.Code != b.Code {
			p.add(e.line("code"), e.subject(), "code changed from %s to %s, a released code must never change", b.Code, e.Code)
		}
	}
	for _, e := range current.Errors {
		if b, ok := baseByCode[e.Code]; ok && b.Name != e.Name {
			p.add(e.line("code"), e.subject(), "code %s was used by %s in %s, a released code must never be reused", e.Code, b.Name, base.File)
		}
	}
	return p.err()
}

// loadBase decodes the base catalog. It is not validated: it was valid when released,
// and the validation rules may have changed since.
func loadBase(path string) (*catalog, error) {
	c, p, err := decodeFile(path)
	if err != nil {
		return nil, err
	}
	if err := p.err(); err != nil {
		return nil, err
	}
	return c, nil
}
