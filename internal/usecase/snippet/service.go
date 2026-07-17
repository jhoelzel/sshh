package snippet

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	snippetdomain "shh-h/internal/domain/snippet"
)

type Repository interface {
	LoadSnippets() ([]snippetdomain.Snippet, error)
	SaveSnippets([]snippetdomain.Snippet) error
}

type Service struct {
	mu       sync.RWMutex
	repo     Repository
	snippets []snippetdomain.Snippet
}

func NewService(repo Repository) (*Service, error) {
	items, err := repo.LoadSnippets()
	if err != nil {
		return nil, err
	}
	return &Service{repo: repo, snippets: clone(items)}, nil
}

func (s *Service) List() []snippetdomain.Snippet {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := clone(s.snippets)
	sort.SliceStable(result, func(i, j int) bool {
		leftFolder := strings.ToLower(result[i].Folder)
		rightFolder := strings.ToLower(result[j].Folder)
		if leftFolder != rightFolder {
			return leftFolder < rightFolder
		}
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})
	return result
}

func (s *Service) Create(candidate snippetdomain.Snippet) (snippetdomain.Snippet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, err := newID()
	if err != nil {
		return snippetdomain.Snippet{}, err
	}
	now := time.Now().UTC()
	candidate.ID = id
	candidate.CreatedAt = now
	candidate.UpdatedAt = now
	candidate = candidate.WithDefaults(now)
	if err := candidate.Validate(); err != nil {
		return snippetdomain.Snippet{}, err
	}
	if err := ensureUniqueName(s.snippets, candidate.Name, ""); err != nil {
		return snippetdomain.Snippet{}, err
	}
	next := append(clone(s.snippets), cloneOne(candidate))
	if err := s.repo.SaveSnippets(next); err != nil {
		return snippetdomain.Snippet{}, err
	}
	s.snippets = next
	return cloneOne(candidate), nil
}

func (s *Service) Update(candidate snippetdomain.Snippet) (snippetdomain.Snippet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := indexByID(s.snippets, strings.TrimSpace(candidate.ID))
	if index < 0 {
		return snippetdomain.Snippet{}, errors.New("snippet not found")
	}
	now := time.Now().UTC()
	candidate.ID = s.snippets[index].ID
	candidate.CreatedAt = s.snippets[index].CreatedAt
	candidate.UpdatedAt = now
	candidate = candidate.WithDefaults(now)
	if err := candidate.Validate(); err != nil {
		return snippetdomain.Snippet{}, err
	}
	if err := ensureUniqueName(s.snippets, candidate.Name, candidate.ID); err != nil {
		return snippetdomain.Snippet{}, err
	}
	next := clone(s.snippets)
	next[index] = cloneOne(candidate)
	if err := s.repo.SaveSnippets(next); err != nil {
		return snippetdomain.Snippet{}, err
	}
	s.snippets = next
	return cloneOne(candidate), nil
}

func (s *Service) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := indexByID(s.snippets, strings.TrimSpace(id))
	if index < 0 {
		return errors.New("snippet not found")
	}
	next := append(clone(s.snippets[:index]), clone(s.snippets[index+1:])...)
	if err := s.repo.SaveSnippets(next); err != nil {
		return err
	}
	s.snippets = next
	return nil
}

func (s *Service) Render(id string, values map[string]string) (string, []string, error) {
	s.mu.RLock()
	index := indexByID(s.snippets, strings.TrimSpace(id))
	if index < 0 {
		s.mu.RUnlock()
		return "", nil, errors.New("snippet not found")
	}
	body := s.snippets[index].Body
	s.mu.RUnlock()
	variables, err := snippetdomain.Variables(body)
	if err != nil {
		return "", nil, err
	}
	rendered, err := snippetdomain.Render(body, values)
	if err != nil {
		return "", variables, err
	}
	return rendered, variables, nil
}

func ensureUniqueName(items []snippetdomain.Snippet, name, excludingID string) error {
	key := strings.ToLower(strings.TrimSpace(name))
	for _, item := range items {
		if item.ID != excludingID && strings.ToLower(item.Name) == key {
			return fmt.Errorf("a snippet named %q already exists", strings.TrimSpace(name))
		}
	}
	return nil
}

func indexByID(items []snippetdomain.Snippet, id string) int {
	for index, item := range items {
		if item.ID == id {
			return index
		}
	}
	return -1
}

func clone(items []snippetdomain.Snippet) []snippetdomain.Snippet {
	result := make([]snippetdomain.Snippet, len(items))
	for index, item := range items {
		result[index] = cloneOne(item)
	}
	return result
}

func cloneOne(item snippetdomain.Snippet) snippetdomain.Snippet {
	item.Tags = append([]string(nil), item.Tags...)
	return item
}

func newID() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate snippet id: %w", err)
	}
	return hex.EncodeToString(buffer), nil
}
