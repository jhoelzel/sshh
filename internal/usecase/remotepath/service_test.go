package remotepath

import (
	"testing"

	remotepathdomain "shh-h/internal/domain/remotepath"
)

type memoryRepository struct {
	favorites []remotepathdomain.Favorite
}

func (repository *memoryRepository) LoadFavorites() ([]remotepathdomain.Favorite, error) {
	return clone(repository.favorites), nil
}

func (repository *memoryRepository) SaveFavorites(favorites []remotepathdomain.Favorite) error {
	repository.favorites = clone(favorites)
	return nil
}

func TestServiceCreatesCanonicalFavoriteAndDeletesIt(t *testing.T) {
	service, err := NewService(&memoryRepository{})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	created, err := service.Create(" profile ", "/srv/app/../logs")
	if err != nil {
		t.Fatalf("create favorite: %v", err)
	}
	if created.ProfileID != "profile" || created.Path != "/srv/logs" || created.ID == "" {
		t.Fatalf("unexpected favorite: %#v", created)
	}
	if err := service.Delete(created.ID); err != nil {
		t.Fatalf("delete favorite: %v", err)
	}
	if len(service.List()) != 0 {
		t.Fatal("expected favorite to be deleted")
	}
}

func TestServiceRejectsDuplicateProfilePath(t *testing.T) {
	service, err := NewService(&memoryRepository{})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if _, err := service.Create("profile", "/srv/logs"); err != nil {
		t.Fatalf("create first favorite: %v", err)
	}
	if _, err := service.Create("profile", "/srv/./logs"); err == nil {
		t.Fatal("expected canonical duplicate rejection")
	}
	if _, err := service.Create("other-profile", "/srv/logs"); err != nil {
		t.Fatalf("same path should be valid for another profile: %v", err)
	}
}
