package workspace

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	workspacedomain "shh-h/internal/domain/workspace"
)

const MaxLayouts = 64

type Repository interface {
	LoadLayouts() ([]workspacedomain.Layout, error)
	SaveLayouts([]workspacedomain.Layout) error
}

type Service struct {
	mu      sync.RWMutex
	repo    Repository
	layouts []workspacedomain.Layout
}

func NewService(repo Repository) (*Service, error) {
	layouts, err := repo.LoadLayouts()
	if err != nil {
		return nil, err
	}
	return &Service{repo: repo, layouts: clone(layouts)}, nil
}

func (service *Service) List() []workspacedomain.Layout {
	service.mu.RLock()
	defer service.mu.RUnlock()
	result := clone(service.layouts)
	sort.SliceStable(result, func(i, j int) bool {
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})
	return result
}

func (service *Service) Create(candidate workspacedomain.Layout) (workspacedomain.Layout, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if len(service.layouts) >= MaxLayouts {
		return workspacedomain.Layout{}, fmt.Errorf("cannot save more than %d workspace layouts", MaxLayouts)
	}
	id, err := newID()
	if err != nil {
		return workspacedomain.Layout{}, err
	}
	now := time.Now().UTC()
	candidate.ID = id
	candidate.CreatedAt = now
	candidate.UpdatedAt = now
	candidate = candidate.WithDefaults(now)
	if err := candidate.Validate(); err != nil {
		return workspacedomain.Layout{}, err
	}
	if err := ensureUniqueName(service.layouts, candidate.Name, ""); err != nil {
		return workspacedomain.Layout{}, err
	}
	next := append(clone(service.layouts), cloneOne(candidate))
	if err := service.repo.SaveLayouts(next); err != nil {
		return workspacedomain.Layout{}, err
	}
	service.layouts = next
	return cloneOne(candidate), nil
}

func (service *Service) Update(candidate workspacedomain.Layout) (workspacedomain.Layout, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	index := indexByID(service.layouts, strings.TrimSpace(candidate.ID))
	if index < 0 {
		return workspacedomain.Layout{}, errors.New("workspace layout not found")
	}
	now := time.Now().UTC()
	candidate.ID = service.layouts[index].ID
	candidate.CreatedAt = service.layouts[index].CreatedAt
	candidate.UpdatedAt = now
	candidate = candidate.WithDefaults(now)
	if err := candidate.Validate(); err != nil {
		return workspacedomain.Layout{}, err
	}
	if err := ensureUniqueName(service.layouts, candidate.Name, candidate.ID); err != nil {
		return workspacedomain.Layout{}, err
	}
	next := clone(service.layouts)
	next[index] = cloneOne(candidate)
	if err := service.repo.SaveLayouts(next); err != nil {
		return workspacedomain.Layout{}, err
	}
	service.layouts = next
	return cloneOne(candidate), nil
}

func (service *Service) Delete(id string) error {
	service.mu.Lock()
	defer service.mu.Unlock()
	index := indexByID(service.layouts, strings.TrimSpace(id))
	if index < 0 {
		return errors.New("workspace layout not found")
	}
	next := append(clone(service.layouts[:index]), clone(service.layouts[index+1:])...)
	if err := service.repo.SaveLayouts(next); err != nil {
		return err
	}
	service.layouts = next
	return nil
}

func ensureUniqueName(layouts []workspacedomain.Layout, name, excludingID string) error {
	key := strings.ToLower(strings.TrimSpace(name))
	for _, layout := range layouts {
		if layout.ID != excludingID && strings.ToLower(layout.Name) == key {
			return fmt.Errorf("a workspace layout named %q already exists", strings.TrimSpace(name))
		}
	}
	return nil
}

func indexByID(layouts []workspacedomain.Layout, id string) int {
	for index, layout := range layouts {
		if layout.ID == id {
			return index
		}
	}
	return -1
}

func clone(layouts []workspacedomain.Layout) []workspacedomain.Layout {
	result := make([]workspacedomain.Layout, len(layouts))
	for index, layout := range layouts {
		result[index] = cloneOne(layout)
	}
	return result
}

func cloneOne(layout workspacedomain.Layout) workspacedomain.Layout {
	layout.Tabs = append([]workspacedomain.Tab(nil), layout.Tabs...)
	return layout
}

func newID() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate workspace layout id: %w", err)
	}
	return hex.EncodeToString(buffer), nil
}
