// Package web holds the built dashboard. `make web` builds it into dist,
// which is embedded in the client binary.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Files returns the built dashboard. It holds only a placeholder until
// `make web` has run.
func Files() fs.FS {
	files, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return files
}
