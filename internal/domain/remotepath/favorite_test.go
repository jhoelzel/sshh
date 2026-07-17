package remotepath

import (
	"strings"
	"testing"
	"time"
)

func TestFavoriteDefaultsCanonicalizeAbsolutePath(t *testing.T) {
	now := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	favorite := (Favorite{ID: " favorite ", ProfileID: " profile ", Path: "/srv/app/../logs"}).WithDefaults(now)
	if favorite.ID != "favorite" || favorite.ProfileID != "profile" || favorite.Path != "/srv/logs" {
		t.Fatalf("unexpected defaults: %#v", favorite)
	}
	if err := favorite.Validate(); err != nil {
		t.Fatalf("validate favorite: %v", err)
	}
}

func TestFavoritePreservesSignificantPathSpaces(t *testing.T) {
	favorite := (Favorite{ID: "favorite", ProfileID: "profile", Path: "/srv/logs "}).WithDefaults(time.Now())
	if favorite.Path != "/srv/logs " {
		t.Fatalf("path spaces were changed: %q", favorite.Path)
	}
	if err := favorite.Validate(); err != nil {
		t.Fatalf("validate path with spaces: %v", err)
	}
}

func TestFavoriteRejectsRelativeAndUnsafePaths(t *testing.T) {
	base := Favorite{ID: "favorite", ProfileID: "profile", CreatedAt: time.Now()}
	for _, remotePath := range []string{"logs", "/srv\x00logs", "/" + strings.Repeat("a", maxPathLength)} {
		candidate := base
		candidate.Path = remotePath
		if err := candidate.Validate(); err == nil {
			t.Fatalf("expected path %q to be rejected", remotePath)
		}
	}
}
