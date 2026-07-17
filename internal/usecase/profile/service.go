package profile

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"shh-h/internal/apperror"
	profiledomain "shh-h/internal/domain/profile"
)

type Repository interface {
	LoadProfiles() ([]profiledomain.Profile, error)
	SaveProfiles([]profiledomain.Profile) error
}

type Service struct {
	mu       sync.RWMutex
	repo     Repository
	profiles []profiledomain.Profile
}

func NewService(repo Repository) (*Service, error) {
	profiles, err := repo.LoadProfiles()
	if err != nil {
		return nil, err
	}
	return &Service{repo: repo, profiles: cloneProfiles(profiles)}, nil
}

func (s *Service) List() []profiledomain.Profile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneProfiles(s.profiles)
}

func (s *Service) Find(id string) (profiledomain.Profile, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, item := range s.profiles {
		if item.ID == id {
			return cloneProfile(item), true
		}
	}
	return profiledomain.Profile{}, false
}

func (s *Service) Create(candidate profiledomain.Profile) (profiledomain.Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id, err := newID()
	if err != nil {
		return profiledomain.Profile{}, err
	}
	now := time.Now().UTC()
	candidate.ID = id
	candidate.CreatedAt = now
	candidate.UpdatedAt = now
	candidate = candidate.WithDefaults(now)
	if err := candidate.Validate(); err != nil {
		return profiledomain.Profile{}, apperror.Wrap(
			apperror.CodeInvalidArgument, "create profile", err.Error(), err,
		)
	}
	if err := ensureUniqueName(s.profiles, candidate.Name, ""); err != nil {
		return profiledomain.Profile{}, err
	}

	next := append(cloneProfiles(s.profiles), cloneProfile(candidate))
	if err := s.repo.SaveProfiles(next); err != nil {
		return profiledomain.Profile{}, err
	}
	s.profiles = next
	return cloneProfile(candidate), nil
}

func (s *Service) Update(candidate profiledomain.Profile) (profiledomain.Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	index := indexByID(s.profiles, strings.TrimSpace(candidate.ID))
	if index < 0 {
		return profiledomain.Profile{}, apperror.New(apperror.CodeNotFound, "Profile was not found.")
	}
	now := time.Now().UTC()
	candidate.ID = s.profiles[index].ID
	candidate.CreatedAt = s.profiles[index].CreatedAt
	candidate.UpdatedAt = now
	candidate = candidate.WithDefaults(now)
	if err := candidate.Validate(); err != nil {
		return profiledomain.Profile{}, apperror.Wrap(
			apperror.CodeInvalidArgument, "update profile", err.Error(), err,
		)
	}
	if err := ensureUniqueName(s.profiles, candidate.Name, candidate.ID); err != nil {
		return profiledomain.Profile{}, err
	}

	next := cloneProfiles(s.profiles)
	next[index] = cloneProfile(candidate)
	if err := s.repo.SaveProfiles(next); err != nil {
		return profiledomain.Profile{}, err
	}
	s.profiles = next
	return cloneProfile(candidate), nil
}

func (s *Service) Duplicate(id string) (profiledomain.Profile, error) {
	s.mu.RLock()
	index := indexByID(s.profiles, id)
	if index < 0 {
		s.mu.RUnlock()
		return profiledomain.Profile{}, apperror.New(apperror.CodeNotFound, "Profile was not found.")
	}
	candidate := cloneProfile(s.profiles[index])
	existing := cloneProfiles(s.profiles)
	s.mu.RUnlock()

	candidate.ID = ""
	candidate.Name = availableCopyName(existing, candidate.Name)
	candidate.CreatedAt = time.Time{}
	candidate.UpdatedAt = time.Time{}
	return s.Create(candidate)
}

func (s *Service) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	index := indexByID(s.profiles, id)
	if index < 0 {
		return apperror.New(apperror.CodeNotFound, "Profile was not found.")
	}
	next := append(cloneProfiles(s.profiles[:index]), cloneProfiles(s.profiles[index+1:])...)
	if err := s.repo.SaveProfiles(next); err != nil {
		return err
	}
	s.profiles = next
	return nil
}

// Import validates and persists all candidates as one profile snapshot. IDs and
// timestamps from external formats are deliberately ignored.
func (s *Service) Import(candidates []profiledomain.Profile) ([]profiledomain.Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(candidates) == 0 {
		return []profiledomain.Profile{}, nil
	}

	next := cloneProfiles(s.profiles)
	imported := make([]profiledomain.Profile, 0, len(candidates))
	now := time.Now().UTC()
	for index, source := range candidates {
		id, err := newID()
		if err != nil {
			return nil, err
		}

		candidate := cloneProfile(source)
		candidate.ID = id
		candidate.CreatedAt = now
		candidate.UpdatedAt = now
		candidate = candidate.WithDefaults(now)
		candidate.Name = availableImportName(next, candidate.Name)
		if err := candidate.Validate(); err != nil {
			return nil, apperror.Wrap(
				apperror.CodeInvalidArgument,
				"import profile",
				fmt.Sprintf("Profile %d (%q) is invalid: %s", index+1, candidate.Name, err),
				err,
			)
		}

		next = append(next, cloneProfile(candidate))
		imported = append(imported, cloneProfile(candidate))
	}

	if err := s.repo.SaveProfiles(next); err != nil {
		return nil, err
	}
	s.profiles = next
	return imported, nil
}

func ensureUniqueName(profiles []profiledomain.Profile, name, excludingID string) error {
	key := strings.ToLower(strings.TrimSpace(name))
	for _, item := range profiles {
		if item.ID != excludingID && strings.ToLower(item.Name) == key {
			return apperror.New(
				apperror.CodeConflict, fmt.Sprintf("A profile named %q already exists.", strings.TrimSpace(name)),
			)
		}
	}
	return nil
}

func availableCopyName(profiles []profiledomain.Profile, source string) string {
	base := "Copy of " + source
	for suffix := 1; ; suffix++ {
		candidate := base
		if suffix > 1 {
			candidate = fmt.Sprintf("%s (%d)", base, suffix)
		}
		if ensureUniqueName(profiles, candidate, "") == nil {
			return candidate
		}
	}
}

func availableImportName(profiles []profiledomain.Profile, source string) string {
	source = strings.TrimSpace(source)
	if ensureUniqueName(profiles, source, "") == nil {
		return source
	}

	base := source + " (imported)"
	for suffix := 1; ; suffix++ {
		candidate := base
		if suffix > 1 {
			candidate = fmt.Sprintf("%s (%d)", base, suffix)
		}
		if ensureUniqueName(profiles, candidate, "") == nil {
			return candidate
		}
	}
}

func indexByID(profiles []profiledomain.Profile, id string) int {
	for index, item := range profiles {
		if item.ID == id {
			return index
		}
	}
	return -1
}

func cloneProfiles(profiles []profiledomain.Profile) []profiledomain.Profile {
	result := make([]profiledomain.Profile, len(profiles))
	for i, item := range profiles {
		result[i] = cloneProfile(item)
	}
	return result
}

func cloneProfile(item profiledomain.Profile) profiledomain.Profile {
	item.Arguments = append([]string(nil), item.Arguments...)
	item.Tags = append([]string(nil), item.Tags...)
	if item.Environment != nil {
		environment := make(map[string]string, len(item.Environment))
		for key, value := range item.Environment {
			environment[key] = value
		}
		item.Environment = environment
	}
	return item
}

func newID() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate profile id: %w", err)
	}
	return hex.EncodeToString(buffer), nil
}
