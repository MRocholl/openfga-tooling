package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	"github.com/tliron/commonlog"
	glspserver "github.com/tliron/glsp/server"

	_ "github.com/tliron/commonlog/simple"

	"github.com/MRocholl/openfga-tooling/fga-lsp/internal/agent"
	"github.com/MRocholl/openfga-tooling/fga-lsp/internal/server"
)

func main() {
	if len(os.Args) > 1 {
		if os.Args[1] == "mcp" {
			runMCP(os.Args[2:])

			return
		}

		if cmd, ok := lookup(os.Args[1]); ok {
			runCommand(cmd, os.Args[2:])

			return
		}

		if os.Args[1] == "help" || os.Args[1] == "-h" || os.Args[1] == "--help" {
			usage()

			return
		}

		// Anything that is not a flag was meant as a subcommand. Falling
		// through would start the language server, which then waits on stdin
		// for an LSP client that is never coming, and looks like a hang.
		if !strings.HasPrefix(os.Args[1], "-") {
			fmt.Fprintf(os.Stderr, "fga-lsp: unknown command %q\n\n", os.Args[1])
			usage()
			os.Exit(2)
		}
	}

	var (
		showVersion = flag.Bool("version", false, "print the version and exit")
		logPath     = flag.String("log", "", "write logs to this file (default: discard)")
		verbosity   = flag.Int("verbosity", 0, "log verbosity, 0-2")
		tcpAddress  = flag.String("tcp", "", "serve over TCP on this address instead of stdio")
	)

	flag.Parse()

	version := buildVersion()

	if *showVersion {
		fmt.Println(server.Name, version) //nolint:forbidigo // this is the program's output

		return
	}

	configureLogging(*logPath, *verbosity)

	fga := server.New(version)
	lsp := glspserver.NewServer(fga.Handler(), server.Name, *logPath != "")

	var err error

	if *tcpAddress != "" {
		err = lsp.RunTCP(*tcpAddress)
	} else {
		err = lsp.RunStdio()
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, server.Name+": "+err.Error())
		os.Exit(1)
	}
}

// configureLogging keeps every log line away from stdout, which carries the protocol.
func configureLogging(path string, verbosity int) {
	if path == "" {
		commonlog.Configure(-4, nil)

		return
	}

	commonlog.Configure(verbosity, &path)
}

func buildVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" {
		return "dev"
	}

	return info.Main.Version
}

// runMCP serves the agent-facing tools over stdio, rooted at the given
// directory or the working directory.
func runMCP(args []string) {
	flags := flag.NewFlagSet("mcp", flag.ExitOnError)
	logPath := flags.String("log", "", "write logs to this file (default: discard)")

	_ = flags.Parse(args)

	root := "."
	if flags.NArg() > 0 {
		root = flags.Arg(0)
	}

	configureLogging(*logPath, 0)

	workspace, err := agent.Open(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, server.Name+": "+err.Error())
		os.Exit(1)
	}

	if err := agent.ServeMCP(context.Background(), workspace, buildVersion()); err != nil {
		fmt.Fprintln(os.Stderr, server.Name+": "+err.Error())
		os.Exit(1)
	}
}
