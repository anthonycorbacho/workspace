// Command errgen generates the coded error constructors and their documentation from a YAML error catalog.
//
// Usage:
//
//	errgen -input errors.defs.yaml -output error.gen.go -docs docs [-package errors] [-base old.yaml] [-check]
//
// The generated constructors return a *Error of the package, they must be generated in the
// package defining Error (kit/errors). The docs dir is owned by errgen: its .md files are replaced.
//
// With -base, the codes are checked against an older version of the catalog: a released code
// must never change nor be reused. With -check, nothing is written: the catalog is validated and the
// output and docs are compared with what would be generated (the comparison is skipped
// with -base when -output and -docs are omitted).
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

type config struct {
	input  string
	output string
	docs   string
	pkg    string
	base   string
	check  bool
}

func main() {
	var cfg config
	flag.StringVar(&cfg.input, "input", "", "YAML error catalog (required)")
	flag.StringVar(&cfg.output, "output", "", "generated Go file")
	flag.StringVar(&cfg.docs, "docs", "", "generated markdown docs dir, its .md files are replaced")
	flag.StringVar(&cfg.pkg, "package", "errors", "package of the generated Go file")
	flag.StringVar(&cfg.base, "base", "", "older version of the catalog (eg: from main) to check that released codes are unchanged")
	flag.BoolVar(&cfg.check, "check", false, "write nothing, fail if the catalog is invalid or the output and docs are not up to date")
	flag.Parse()

	if err := cfg.validate(); err != nil {
		fmt.Fprintf(os.Stderr, "errgen: %v\n", err)
		flag.Usage()
		os.Exit(2)
	}
	if err := run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func (cfg config) validate() error {
	if cfg.input == "" {
		return errors.New("-input is required")
	}
	if !token.IsIdentifier(cfg.pkg) {
		return fmt.Errorf("-package %q is not a valid package name", cfg.pkg)
	}
	// -check -base alone is the stability check of a pull request, there is nothing to compare.
	if cfg.check && cfg.base != "" {
		return nil
	}
	if cfg.output == "" || cfg.docs == "" {
		return errors.New("-output and -docs are required")
	}
	return nil
}

func run(cfg config) error {
	c, err := load(cfg.input)
	if err != nil {
		return err
	}
	if cfg.base != "" {
		base, err := loadBase(cfg.base)
		if err != nil {
			return err
		}
		if err := checkStability(base, c); err != nil {
			return err
		}
	}
	if cfg.check && cfg.output == "" && cfg.docs == "" {
		return nil
	}

	code, err := generate(c, cfg.pkg)
	if err != nil {
		return err
	}
	pages := renderDocs(c)

	if cfg.check {
		return checkUpToDate(cfg, code, pages)
	}
	// the docs dir is checked first, so that nothing is written if it can't be.
	if _, err := listDocs(cfg.docs); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(cfg.output), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(cfg.output, code, 0o644); err != nil {
		return err
	}
	return writeDocs(cfg.docs, pages)
}

func checkUpToDate(cfg config, code []byte, pages map[string][]byte) error {
	var errs []error
	if cfg.output != "" {
		current, err := os.ReadFile(cfg.output)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if !bytes.Equal(current, code) {
			errs = append(errs, fmt.Errorf("%s is not up to date, run make errgen", filepath.Base(cfg.output)))
		}
	}
	if cfg.docs != "" {
		outdated, err := outdatedDocs(cfg.docs, pages)
		if err != nil {
			return err
		}
		if len(outdated) > 0 {
			errs = append(errs, fmt.Errorf("docs %s are not up to date (%s), run make errgen", cfg.docs, strings.Join(outdated, ", ")))
		}
	}
	return errors.Join(errs...)
}
