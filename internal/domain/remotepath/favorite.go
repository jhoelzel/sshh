package remotepath

import (
	"errors"
	"fmt"
	pathpkg "path"
	"strings"
	"time"
)

const (
	MaxFavorites       = 256
	maxIDLength        = 128
	maxProfileIDLength = 128
	maxPathLength      = 4096
)

type Favorite struct {
	ID        string    `json:"id"`
	ProfileID string    `json:"profileId"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"createdAt"`
}

func (favorite Favorite) WithDefaults(now time.Time) Favorite {
	favorite.ID = strings.TrimSpace(favorite.ID)
	favorite.ProfileID = strings.TrimSpace(favorite.ProfileID)
	if strings.HasPrefix(favorite.Path, "/") {
		favorite.Path = pathpkg.Clean(favorite.Path)
	}
	if favorite.CreatedAt.IsZero() {
		favorite.CreatedAt = now.UTC()
	}
	return favorite
}

func (favorite Favorite) Validate() error {
	if err := validateText("id", favorite.ID, maxIDLength); err != nil {
		return err
	}
	if err := validateText("profile id", favorite.ProfileID, maxProfileIDLength); err != nil {
		return err
	}
	if err := validateText("path", favorite.Path, maxPathLength); err != nil {
		return err
	}
	if !strings.HasPrefix(favorite.Path, "/") {
		return errors.New("remote path favorite path must be absolute")
	}
	if pathpkg.Clean(favorite.Path) != favorite.Path {
		return errors.New("remote path favorite path must be canonical")
	}
	return nil
}

func validateText(field, value string, limit int) error {
	if value == "" {
		return fmt.Errorf("remote path favorite %s is required", field)
	}
	if len(value) > limit {
		return fmt.Errorf("remote path favorite %s exceeds %d bytes", field, limit)
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return fmt.Errorf("remote path favorite %s contains a control character", field)
		}
	}
	return nil
}
