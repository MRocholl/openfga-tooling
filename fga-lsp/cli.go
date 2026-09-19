package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/MRocholl/openfga-tooling/fga-lsp/internal/agent"
)

// command is one agent query, reachable from a shell as well as over MCP.
type command struct {
	name    string
	arg     string
	summary string
	run     func(*agent.Workspace, string) string
}

var commands = []command{
	{"check", "[file]", "validate the models and store tests",
		func(w *agent.Workspace, arg string) string { return w.Check(arg) }},
	{"overview", "", "list the modules, types and conditions",
		func(w *agent.Workspace, _ string) string { return w.Overview() }},
	{"describe", "<type>", "show a type's relations, merged across modules",
		func(w *agent.Workspace, arg string) string { return w.Describe(arg) }},
	{"definition", "<name>", "show where a type or relation is defined",
		func(w *agent.Workspace, arg string) string { return w.Definition(arg) }},
	{"references", "<name>", "find every use of a type or relation",
		func(w *agent.Workspace, arg string) string { return w.References(arg) }},
	{"search", "<query>", "find names matching a query",
		func(w *agent.Workspace, arg string) string { return w.Search(arg) }},
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

	flags.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: fga-lsp %s [-C dir] %s\n", cmd.name, cmd.arg)
		flags.PrintDefaults()
	}

	_ = flags.Parse(args)

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

	out := workspace.Run(cmd.run, arg)
	fmt.Print(out)

	if cmd.name == "check" && !strings.HasPrefix(out, "No problems") {
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

	fmt.Fprint(os.Stderr, "\nEvery query takes -C to pick the directory; it defaults to the current one.\n")
}
