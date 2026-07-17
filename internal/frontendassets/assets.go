package frontendassets

import (
	"embed"
	"io/fs"
)

const (
	productionAssetsDir = "bundle/dist"
	productionIndex     = productionAssetsDir + "/index.html"
	fallbackAssetsDir   = "bundle/fallback"
)

//go:embed all:bundle
var embedded embed.FS

func Assets() (fs.FS, error) {
	return assetsFrom(embedded)
}

// The fallback keeps clean checkouts compilable before Vite creates bundle/dist.
func assetsFrom(source fs.FS) (fs.FS, error) {
	if _, err := fs.Stat(source, productionIndex); err == nil {
		return fs.Sub(source, productionAssetsDir)
	}

	return fs.Sub(source, fallbackAssetsDir)
}
