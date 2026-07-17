package profile

import (
	"errors"
	"testing"

	"shh-h/internal/apperror"
	profiledomain "shh-h/internal/domain/profile"
)

type memoryRepository struct {
	profiles  []profiledomain.Profile
	saveErr   error
	saveCalls int
}

func (r *memoryRepository) LoadProfiles() ([]profiledomain.Profile, error) {
	return cloneProfiles(r.profiles), nil
}

func (r *memoryRepository) SaveProfiles(profiles []profiledomain.Profile) error {
	r.saveCalls++
	if r.saveErr != nil {
		return r.saveErr
	}
	r.profiles = cloneProfiles(profiles)
	return nil
}

func TestServiceImportsProfilesAtomicallyAndRenamesConflicts(t *testing.T) {
	repository := &memoryRepository{profiles: []profiledomain.Profile{
		{ID: "local", Name: "Production", Protocol: profiledomain.ProtocolLocal},
	}}
	service, err := NewService(repository)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}

	imported, err := service.Import([]profiledomain.Profile{
		{ID: "external-id", Name: "Production", Protocol: profiledomain.ProtocolSSH, Host: "one.example.com"},
		{Name: "Production", Protocol: profiledomain.ProtocolSSH, Host: "two.example.com", Username: "deploy"},
	})
	if err != nil {
		t.Fatalf("import profiles: %v", err)
	}
	if repository.saveCalls != 1 {
		t.Fatalf("expected one atomic save, got %d", repository.saveCalls)
	}
	if len(imported) != 2 || imported[0].Name != "Production (imported)" || imported[1].Name != "Production (imported) (2)" {
		t.Fatalf("unexpected imported profiles: %#v", imported)
	}
	if imported[0].ID == "external-id" || imported[0].ID == "" || imported[0].Port != 22 {
		t.Fatalf("external identity or defaults were not handled: %#v", imported[0])
	}
	if imported[0].ID == imported[1].ID {
		t.Fatal("imported profiles share an id")
	}
}

func TestServiceImportRejectsWholeBatchBeforeSave(t *testing.T) {
	repository := &memoryRepository{profiles: []profiledomain.Profile{
		{ID: "local", Name: "Local", Protocol: profiledomain.ProtocolLocal},
	}}
	service, err := NewService(repository)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}

	_, err = service.Import([]profiledomain.Profile{
		{Name: "Valid", Protocol: profiledomain.ProtocolSSH, Host: "valid.example.com"},
		{Name: "Invalid", Protocol: profiledomain.ProtocolSSH, Host: "invalid.example.com", Port: 70000},
	})
	if err == nil {
		t.Fatal("expected invalid batch to fail")
	}
	if !apperror.IsCode(err, apperror.CodeInvalidArgument) {
		t.Fatalf("invalid import error code = %q", apperror.CodeOf(err))
	}
	if repository.saveCalls != 0 || len(service.List()) != 1 {
		t.Fatalf("failed batch changed state: saves=%d profiles=%#v", repository.saveCalls, service.List())
	}
}

func TestServiceImportDoesNotMutateStateWhenPersistenceFails(t *testing.T) {
	repository := &memoryRepository{profiles: []profiledomain.Profile{
		{ID: "local", Name: "Local", Protocol: profiledomain.ProtocolLocal},
	}, saveErr: errors.New("disk full")}
	service, err := NewService(repository)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}

	if _, err := service.Import([]profiledomain.Profile{{Name: "Remote", Protocol: profiledomain.ProtocolSSH, Host: "remote.example.com"}}); err == nil {
		t.Fatal("expected persistence failure")
	}
	if got := service.List(); len(got) != 1 || got[0].ID != "local" {
		t.Fatalf("service state changed after failed import: %#v", got)
	}
}

func TestServiceCRUDAndDuplicate(t *testing.T) {
	repository := &memoryRepository{profiles: []profiledomain.Profile{
		{ID: "local", Name: "Local", Protocol: profiledomain.ProtocolLocal},
	}}
	service, err := NewService(repository)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}

	created, err := service.Create(profiledomain.Profile{
		Name: "Production", Protocol: profiledomain.ProtocolSSH, Host: "example.com", Port: 22,
	})
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if created.ID == "" || created.Authentication != profiledomain.AuthenticationAuto {
		t.Fatalf("unexpected created profile: %#v", created)
	}

	created.Username = "deploy"
	updated, err := service.Update(created)
	if err != nil {
		t.Fatalf("update profile: %v", err)
	}
	if updated.Username != "deploy" || !updated.UpdatedAt.After(updated.CreatedAt) {
		t.Fatalf("unexpected updated profile: %#v", updated)
	}

	duplicated, err := service.Duplicate(created.ID)
	if err != nil {
		t.Fatalf("duplicate profile: %v", err)
	}
	if duplicated.ID == created.ID || duplicated.Name != "Copy of Production" {
		t.Fatalf("unexpected duplicate: %#v", duplicated)
	}

	if err := service.Delete(created.ID); err != nil {
		t.Fatalf("delete profile: %v", err)
	}
	if _, found := service.Find(created.ID); found {
		t.Fatal("deleted profile is still available")
	}
}

func TestServiceDoesNotMutateStateWhenPersistenceFails(t *testing.T) {
	repository := &memoryRepository{profiles: []profiledomain.Profile{
		{ID: "local", Name: "Local", Protocol: profiledomain.ProtocolLocal},
	}}
	service, err := NewService(repository)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	repository.saveErr = errors.New("disk full")

	if _, err := service.Create(profiledomain.Profile{Name: "Other", Protocol: profiledomain.ProtocolLocal}); err == nil {
		t.Fatal("expected create to fail")
	}
	if got := service.List(); len(got) != 1 || got[0].ID != "local" {
		t.Fatalf("service state changed after failed save: %#v", got)
	}
}

func TestServiceRejectsDuplicateName(t *testing.T) {
	service, err := NewService(&memoryRepository{profiles: []profiledomain.Profile{
		{ID: "local", Name: "Local", Protocol: profiledomain.ProtocolLocal},
	}})
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	if _, err := service.Create(profiledomain.Profile{Name: " local ", Protocol: profiledomain.ProtocolLocal}); !apperror.IsCode(err, apperror.CodeConflict) {
		t.Fatalf("duplicate name error code = %q, want %q", apperror.CodeOf(err), apperror.CodeConflict)
	}
}
