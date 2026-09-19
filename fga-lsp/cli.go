package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/MRocholl/openfga-tooling/fga-lsp/internal/agent"
)

// command is one query, reachable from a shell as well as over MCP.
type command struct {
	name    string
	arg     string
	summary string
	run     func(*agent.Workspace, string, agent.Format, int) string
}

var commands = []command{
	{"check", "[file]", "validate the models and store tests",
		func(w *agent.Workspace, arg string, f agent.Format, _ int) string {
			return agent.RenderCheck(w.CheckWorkspace(arg), f)
		}},
	{"overview", "", "list the modules, types and conditions",
		func(w *agent.Workspace, _ string, _ agent.Format, _ int) string { return w.Overview() }},
	{"describe", "<type>", "show a type's relations, merged across modules",
		func(w *agent.Workspace, arg string, _ agent.Format, _ int) string { return w.Describe(arg) }},
	{"definition", "<name>", "show where a type or relation is defined",
		func(w *agent.Workspace, arg string, f agent.Format, _ int) string {
			return agent.RenderDefinition(w.FindDefinition(arg), f)
		}},
	{"references", "<name>", "find every use of a type or relation",
		func(w *agent.Workspace, arg string, f agent.Format, limit int) string {
			return agent.RenderReferences(w.FindReferences(arg, limit), f)
		}},
	{"search", "<query>", "find names matching a query",
		func(w *agent.Workspace, arg string, f agent.Format, limit int) string {
			return agent.RenderSearch(w.FindSymbols(arg, limit), f)
		}},
}

func lookup(name string) (command, bool) {
	for _, c := range commands {
		if c.name == name {
			return c, true
		}
	}

	return command{}, false
}

// runCommand answers one query and exits. `check` exits non-zero when it
// found something, so it composes with a shell and with CI.
func runCommand(cmd command, args []string) {
	flags := flag.NewFlagSet(cmd.name, flag.ExitOnError)
	dir := flags.String("C", ".", "workspace directory to scan")
	format := flags.String("format", string(agent.FormatText),
		"text for a person, agent for one finding per line, json for a machine")
	limit := flags.Int("limit", 0, "cap the number of results; 0 means all of them")

	flags.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: fga-lsp %s [-C dir] [-format text|agent|json] %s\n", cmd.name, cmd.arg)
		flags.PrintDefaults()
	}

	_ = flags.Parse(args)

	chosen, ok := agent.ParseFormat(*format)
	if !ok {
		fmt.Fprintf(os.Stderr, "fga-lsp: unknown format %q; want text, agent or json\n", *format)
		os.Exit(2)
	}

	arg := strings.Join(flags.Args(), " ")
	if arg == "" && strings.HasPrefix(cmd.arg, "<") {
		flags.Usage()
		os.Exit(2)
	}

	workspace, err := agent.Open(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fga-lsp: "+err.Error())
		os.Exit(1)
	}

	workspace.Reload()

	fmt.Print(cmd.run(workspace, arg, chosen, *limit))

	if cmd.name == "check" && !workspace.CheckWorkspace(arg).Clean() {
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `fga-lsp -- OpenFGA models for editors, shells and agents

  fga-lsp                     language server, LSP over stdin/stdout
  fga-lsp mcp [dir]           agent tools, MCP over stdin/stdout

Query a workspace directly:

`)

	for _, c := range commands {
		fmt.Fprintf(os.Stderr, "  fga-lsp %-11s %-9s %s\n", c.name, c.arg, c.summary)
	}

	fmt.Fprint(os.Stderr, `
  -C dir       the workspace to scan, default the current directory
  -format      text (default), agent for one finding per line, or json
  -limit N     cap the results; 0, the default, returns all of them

check exits 1 when it reports something, 0 when clean.
`)
}
