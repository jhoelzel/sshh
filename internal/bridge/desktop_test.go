package bridge

import (
	"strings"
	"testing"
	"time"

	profiledomain "shh-h/internal/domain/profile"
	remotepathdomain "shh-h/internal/domain/remotepath"
	settingsdomain "shh-h/internal/domain/settings"
	profileusecase "shh-h/internal/usecase/profile"
	remotepathusecase "shh-h/internal/usecase/remotepath"
	sessionusecase "shh-h/internal/usecase/session"
)

type bridgeProfileRepository struct {
	profiles []profiledomain.Profile
}

func (repository *bridgeProfileRepository) LoadProfiles() ([]profiledomain.Profile, error) {
	return repository.profiles, nil
}

func (repository *bridgeProfileRepository) SaveProfiles(profiles []profiledomain.Profile) error {
	repository.profiles = profiles
	return nil
}

type bridgeRemotePathRepository struct {
	favorites []remotepathdomain.Favorite
}

func (repository *bridgeRemotePathRepository) LoadFavorites() ([]remotepathdomain.Favorite, error) {
	return repository.favorites, nil
}

func (repository *bridgeRemotePathRepository) SaveFavorites(favorites []remotepathdomain.Favorite) error {
	repository.favorites = favorites
	return nil
}

func TestAttachFrontendIsIdempotentForSameInstance(t *testing.T) {
	desktop := NewDesktop(sessionusecase.NewManager(nil), nil, nil, nil, nil, nil, nil, nil, nil, nil)

	first, err := desktop.AttachFrontend("frontend-instance")
	if err != nil {
		t.Fatalf("attach frontend: %v", err)
	}
	second, err := desktop.AttachFrontend("frontend-instance")
	if err != nil {
		t.Fatalf("reattach frontend: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("same frontend instance received a new lease: %q != %q", first.ID, second.ID)
	}
	if _, err := time.Parse(time.RFC3339Nano, second.ExpiresAt); err != nil {
		t.Fatalf("lease expiry is not RFC3339: %v", err)
	}
}

func TestAttachFrontendReplacesPreviousInstance(t *testing.T) {
	desktop := NewDesktop(sessionusecase.NewManager(nil), nil, nil, nil, nil, nil, nil, nil, nil, nil)

	first, err := desktop.AttachFrontend("first-instance")
	if err != nil {
		t.Fatalf("attach first frontend: %v", err)
	}
	second, err := desktop.AttachFrontend("second-instance")
	if err != nil {
		t.Fatalf("attach second frontend: %v", err)
	}
	if first.ID == second.ID {
		t.Fatal("replacement frontend reused the previous lease")
	}
	if _, err := desktop.RenewFrontendLease(first.ID); err == nil {
		t.Fatal("expected the replaced lease to be rejected")
	}
	if _, err := desktop.RenewFrontendLease(second.ID); err != nil {
		t.Fatalf("renew active lease: %v", err)
	}
}

func TestAttachFrontendRejectsInvalidNonce(t *testing.T) {
	desktop := NewDesktop(sessionusecase.NewManager(nil), nil, nil, nil, nil, nil, nil, nil, nil, nil)

	if _, err := desktop.AttachFrontend("  "); err == nil {
		t.Fatal("expected an empty frontend nonce to be rejected")
	}
}

func TestTerminalTextFilenameSanitizesUntrustedTitles(t *testing.T) {
	filename := terminalTextFilename(" ../../Production\nShell ")
	if filename != "Production-Shell-selection.txt" {
		t.Fatalf("unexpected filename %q", filename)
	}
	if fallback := terminalTextFilename("///"); fallback != "terminal-selection.txt" {
		t.Fatalf("unexpected fallback filename %q", fallback)
	}
	if long := terminalTextFilename(strings.Repeat("界", 100)); len(long) > 100 {
		t.Fatalf("suggested filename exceeds byte budget: %d", len(long))
	}
}

func TestSettingsDTORoundTripIncludesNotifications(t *testing.T) {
	settings := settingsdomain.Defaults()
	settings.Notifications.Enabled = true
	settings.Notifications.LongTransferSeconds = 75
	if roundTrip := settingsFromDTO(settingsDTO(settings)); roundTrip != settings {
		t.Fatalf("settings DTO changed notification preferences: %#v", roundTrip)
	}
}

func TestRemotePathFavoritesRequireExistingSSHProfile(t *testing.T) {
	profiles, err := profileusecase.NewService(&bridgeProfileRepository{profiles: []profiledomain.Profile{
		{ID: "local", Protocol: profiledomain.ProtocolLocal},
		{ID: "ssh", Protocol: profiledomain.ProtocolSSH},
	}})
	if err != nil {
		t.Fatalf("new profile service: %v", err)
	}
	remotePaths, err := remotepathusecase.NewService(&bridgeRemotePathRepository{})
	if err != nil {
		t.Fatalf("new remote path service: %v", err)
	}
	desktop := NewDesktop(sessionusecase.NewManager(nil), profiles, nil, nil, nil, nil, nil, remotePaths, nil, nil)

	if _, err := desktop.CreateRemotePathFavorite("missing", "/srv/app"); err == nil {
		t.Fatal("expected missing profile rejection")
	}
	if _, err := desktop.CreateRemotePathFavorite("local", "/srv/app"); err == nil {
		t.Fatal("expected local profile rejection")
	}
	created, err := desktop.CreateRemotePathFavorite("ssh", "/srv/app/../logs")
	if err != nil {
		t.Fatalf("create SSH favorite: %v", err)
	}
	if created.ProfileID != "ssh" || created.Path != "/srv/logs" || len(desktop.ListRemotePathFavorites()) != 1 {
		t.Fatalf("unexpected remote path favorite: %#v", created)
	}
}
