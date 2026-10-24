// Command codegen scaffolds a new project (service) in a namespace of the workspace, see the codegen package.
//
//	make codegen
//	make codegen ARGS="-namespace sample -project billing"
//
// The values not given as flags are prompted when the command runs in a terminal.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/anthonycorbacho/workspace/sdlc/codegen"
	"github.com/anthonycorbacho/workspace/sdlc/codegen/namespace"
	"github.com/anthonycorbacho/workspace/sdlc/codegen/project"
	"github.com/charmbracelet/huh"
	"golang.org/x/term"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, isTerminal(os.Stdin)); err != nil {
		fmt.Fprintln(os.Stderr, "codegen:", err)
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(2)
		}
		os.Exit(1)
	}
}

type options struct {
	root      string
	namespace string
	project   string
	dryRun    bool
	force     bool
	yes       bool
}

func run(args []string, out io.Writer, interactive bool) error {
	opts, err := parseFlags(args)
	if err != nil {
		return err
	}
	root, err := filepath.Abs(opts.root)
	if err != nil {
		return err
	}
	module, err := codegen.ModulePath(root)
	if err != nil {
		return err
	}

	if opts.namespace == "" || opts.project == "" {
		if !interactive {
			return errors.New("-namespace and -project are required when not running in a terminal")
		}
		if err := prompt(root, &opts); err != nil {
			return err
		}
	}
	if err := validate(root, opts); err != nil {
		return err
	}

	state := codegen.State{Root: root, Module: module, Namespace: opts.namespace, Project: opts.project}
	ws := codegen.NewWorkspace(root, opts.force)
	for _, step := range []codegen.Step{namespace.Step{}, project.Step{}} {
		if err := step.Generate(state, ws); err != nil {
			return err
		}
	}
	return write(ws, opts, out, interactive)
}

func parseFlags(args []string) (options, error) {
	var opts options
	fs := flag.NewFlagSet("codegen", flag.ContinueOnError)
	fs.StringVar(&opts.root, "root", ".", "root directory of the workspace (where go.mod is)")
	fs.StringVar(&opts.namespace, "namespace", "", "namespace of the project, created if it doesn't exist (eg: sample)")
	fs.StringVar(&opts.project, "project", "", "name of the project to create (eg: sampleapp)")
	fs.BoolVar(&opts.dryRun, "dry-run", false, "print the files that would be created or updated, without writing them")
	fs.BoolVar(&opts.force, "force", false, "overwrite the project files that already exist")
	fs.BoolVar(&opts.yes, "yes", false, "don't ask for confirmation before writing the files")
	return opts, fs.Parse(args)
}

// write prints the changes and writes them, after a confirmation when running in a terminal.
func write(ws *codegen.Workspace, opts options, out io.Writer, interactive bool) error {
	if opts.dryRun {
		fmt.Fprintln(out, "Files that would be written (dry run):")
		ws.Print(out)
		return nil
	}
	fmt.Fprintln(out, "Files to write:")
	ws.Print(out)
	if interactive && !opts.yes {
		confirmed := true
		if err := huh.NewConfirm().Title("Write these files?").Value(&confirmed).Run(); err != nil {
			return err
		}
		if !confirmed {
			fmt.Fprintln(out, "Nothing written.")
			return nil
		}
	}
	if err := ws.Commit(); err != nil {
		return err
	}

	fmt.Fprintf(out, "\n%s/%s created, next steps:\n", opts.namespace, opts.project)
	fmt.Fprintf(out, "  - build the service from %s/%s/cmd/%s/main.go (see sample/sampleapp)\n", opts.namespace, opts.project, opts.project)
	fmt.Fprintln(out, "  - run it locally with `tilt up`")
	return nil
}

// prompt asks the values not given as flags.
func prompt(root string, opts *options) error {
	var fields []huh.Field
	if opts.namespace == "" {
		existing := codegen.Namespaces(root)
		fields = append(fields, huh.NewInput().
			Title("Namespace").
			Description("existing: "+strings.Join(existing, ", ")+" (a new namespace is created)").
			Suggestions(existing).
			Value(&opts.namespace).
			Validate(func(s string) error { return codegen.ValidateName("namespace", s) }))
	}
	if opts.project == "" {
		fields = append(fields, huh.NewInput().
			Title("Project").
			Description("name of the service, eg: billing").
			Value(&opts.project).
			Validate(func(s string) error { return codegen.ValidateName("project", s) }))
	}
	return huh.NewForm(huh.NewGroup(fields...)).Run()
}

func validate(root string, opts options) error {
	if err := codegen.ValidateName("namespace", opts.namespace); err != nil {
		return err
	}
	if err := codegen.ValidateName("project", opts.project); err != nil {
		return err
	}
	if ns := codegen.ProjectNamespace(root, opts.project); ns != "" && ns != opts.namespace {
		return fmt.Errorf("project %q already exists in the namespace %q, project names are unique in the workspace", opts.project, ns)
	}
	return nil
}

// isTerminal reports whether f is an interactive terminal (eg: not a pipe nor /dev/null in CI).
func isTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}
