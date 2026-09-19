package analysis

import (
	"net/url"
	"path/filepath"
	"runtime"
	"strings"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

// PathFromURI converts a file:// URI into a local path.
func PathFromURI(uri protocol.DocumentUri) string {
	parsed, err := url.Parse(string(uri))
	if err != nil || parsed.Scheme != "file" {
		return ""
	}

	path := parsed.Path

	if runtime.GOOS == "windows" {
		path = strings.TrimPrefix(path, "/")
		path = filepath.FromSlash(path)
	}

	return filepath.Clean(path)
}

func URIFromPath(path string) protocol.DocumentUri {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}

	slashed := filepath.ToSlash(abs)
	if !strings.HasPrefix(slashed, "/") {
		slashed = "/" + slashed
	}

	uri := url.URL{Scheme: "file", Path: slashed}

	return protocol.DocumentUri(uri.String())
}
