package main

import (
	"flag"
	"fmt"
	"os"
	"runtime/debug"

	"github.com/tliron/commonlog"
	glspserver "github.com/tliron/glsp/server"

	_ "github.com/tliron/commonlog/simple"

	"github.com/MRocholl/openfga-tooling/fga-lsp/internal/server"
)

func main() {
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
