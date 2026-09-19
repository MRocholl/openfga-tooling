package analysis

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

// Scan loads every model, store test and fga.mod under root into the index.
func Scan(index *Index, root string) int {
	loaded := 0

	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr
		}

		if entry.IsDir() {
			if skipDir(entry.Name()) {
				return fs.SkipDir
			}

			return nil
		}

		if KindOf(path) == KindUnknown {
			return nil
		}

		content, err := os.ReadFile(path) //nolint:gosec
		if err != nil {
			return nil //nolint:nilerr
		}

		index.Put(URIFromPath(path), 0, content)

		loaded++

		return nil
	})

	return loaded
}

func skipDir(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", ".venv", "target", "dist", "build":
		return true
	}

	return strings.HasPrefix(name, ".") && name != "."
}

// URIsUnder lists the indexed documents whose path lies under root.
func URIsUnder(v *View, root string) []protocol.DocumentUri {
	var out []protocol.DocumentUri

	for _, doc := range v.Documents() {
		if strings.HasPrefix(doc.Path, filepath.Clean(root)) {
			out = append(out, doc.URI)
		}
	}

	return out
}
