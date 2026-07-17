package remotepath

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"shh-h/internal/apperror"
	remotepathdomain "shh-h/internal/domain/remotepath"
)

type Repository interface {
	LoadFavorites() ([]remotepathdomain.Favorite, error)
	SaveFavorites([]remotepathdomain.Favorite) error
}

type Service struct {
	mu        sync.RWMutex
	repo      Repository
	favorites []remotepathdomain.Favorite
}

func NewService(repo Repository) (*Service, error) {
	favorites, err := repo.LoadFavorites()
	if err != nil {
		return nil, err
	}
	return &Service{repo: repo, favorites: clone(favorites)}, nil
}

func (service *Service) List() []remotepathdomain.Favorite {
	service.mu.RLock()
	defer service.mu.RUnlock()
	result := clone(service.favorites)
	sort.SliceStable(result, func(left, right int) bool {
		if result[left].ProfileID != result[right].ProfileID {
			return result[left].ProfileID < result[right].ProfileID
		}
		return strings.ToLower(result[left].Path) < strings.ToLower(result[right].Path)
	})
	return result
}

func (service *Service) Create(profileID, remotePath string) (remotepathdomain.Favorite, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if len(service.favorites) >= remotepathdomain.MaxFavorites {
		return remotepathdomain.Favorite{}, apperror.New(
			apperror.CodeConflict,
			fmt.Sprintf("Cannot save more than %d remote path favorites.", remotepathdomain.MaxFavorites),
		)
	}
	id, err := newID()
	if err != nil {
		return remotepathdomain.Favorite{}, err
	}
	candidate := (remotepathdomain.Favorite{
		ID: id, ProfileID: profileID, Path: remotePath,
	}).WithDefaults(time.Now().UTC())
	if err := candidate.Validate(); err != nil {
		return remotepathdomain.Favorite{}, apperror.Wrap(
			apperror.CodeInvalidArgument, "create remote path favorite", err.Error(), err,
		)
	}
	for _, favorite := range service.favorites {
		if favorite.ProfileID == candidate.ProfileID && favorite.Path == candidate.Path {
			return remotepathdomain.Favorite{}, apperror.New(
				apperror.CodeConflict, "Remote path is already a favorite for this profile.",
			)
		}
	}
	next := append(clone(service.favorites), candidate)
	if err := service.repo.SaveFavorites(next); err != nil {
		return remotepathdomain.Favorite{}, err
	}
	service.favorites = next
	return candidate, nil
}

func (service *Service) Delete(id string) error {
	service.mu.Lock()
	defer service.mu.Unlock()
	id = strings.TrimSpace(id)
	index := -1
	for candidateIndex, favorite := range service.favorites {
		if favorite.ID == id {
			index = candidateIndex
			break
		}
	}
	if index < 0 {
		return apperror.New(apperror.CodeNotFound, "Remote path favorite was not found.")
	}
	next := append(clone(service.favorites[:index]), clone(service.favorites[index+1:])...)
	if err := service.repo.SaveFavorites(next); err != nil {
		return err
	}
	service.favorites = next
	return nil
}

func clone(favorites []remotepathdomain.Favorite) []remotepathdomain.Favorite {
	return append([]remotepathdomain.Favorite(nil), favorites...)
}

func newID() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate remote path favorite id: %w", err)
	}
	return hex.EncodeToString(buffer), nil
}
