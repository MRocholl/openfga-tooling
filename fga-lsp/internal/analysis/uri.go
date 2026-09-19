package analysis

import (
	"net/url"
	"path/filepath"
	"runtime"
	"strings"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

// PathFromURI converts a file:// URI into a local path. Anything that is not
// a file URI yields "", which callers treat as "not on disk".
func PathFromURI(uri protocol.DocumentUri) string {
	parsed, err := url.Parse(string(uri))
	if err != nil || parsed.Scheme != "file" {
		return ""
	}

	// url.Parse has already decoded the path. Unescaping it again turns a
	// literal percent into an escape: `/tmp/100%.fga` arrives as
	// `file:///tmp/100%25.fga` and would decode twice into an error, leaving
	// the file silently unanalysed.
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
