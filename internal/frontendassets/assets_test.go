package frontendassets

import (
	"io/fs"
	"testing"
	"testing/fstest"
)

func TestAssetsIncludesIndex(t *testing.T) {
	t.Parallel()

	assets, err := Assets()
	if err != nil {
		t.Fatalf("select embedded assets: %v", err)
	}
	if _, err := fs.Stat(assets, "index.html"); err != nil {
		t.Fatalf("stat embedded index: %v", err)
	}
}

func TestAssetsFromPrefersProductionAssets(t *testing.T) {
	t.Parallel()

	assets, err := assetsFrom(fstest.MapFS{
		"bundle/dist/index.html":     {Data: []byte("production")},
		"bundle/fallback/index.html": {Data: []byte("fallback")},
	})
	if err != nil {
		t.Fatalf("select production assets: %v", err)
	}

	assertIndexContents(t, assets, "production")
}

func TestAssetsFromUsesFallbackWithoutProductionBuild(t *testing.T) {
	t.Parallel()

	assets, err := assetsFrom(fstest.MapFS{
		"bundle/fallback/index.html": {Data: []byte("fallback")},
	})
	if err != nil {
		t.Fatalf("select fallback assets: %v", err)
	}

	assertIndexContents(t, assets, "fallback")
}

func assertIndexContents(t *testing.T, assets fs.FS, want string) {
	t.Helper()

	contents, err := fs.ReadFile(assets, "index.html")
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	if got := string(contents); got != want {
		t.Fatalf("index contents = %q, want %q", got, want)
	}
}
