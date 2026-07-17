package snippet

import (
	"testing"

	snippetdomain "shh-h/internal/domain/snippet"
)

type memoryRepository struct {
	items []snippetdomain.Snippet
}

func (r *memoryRepository) LoadSnippets() ([]snippetdomain.Snippet, error) {
	return clone(r.items), nil
}

func (r *memoryRepository) SaveSnippets(items []snippetdomain.Snippet) error {
	r.items = clone(items)
	return nil
}

func TestServiceCRUDAndRender(t *testing.T) {
	service, err := NewService(&memoryRepository{})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	created, err := service.Create(snippetdomain.Snippet{Name: "Deploy", Folder: "Ops", Body: "deploy {{target}}"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	rendered, variables, err := service.Render(created.ID, map[string]string{"target": "prod"})
	if err != nil || rendered != "deploy prod" || len(variables) != 1 || variables[0] != "target" {
		t.Fatalf("render: text=%q variables=%#v err=%v", rendered, variables, err)
	}
	created.Name = "Release"
	updated, err := service.Update(created)
	if err != nil || updated.Name != "Release" {
		t.Fatalf("update: snippet=%#v err=%v", updated, err)
	}
	if err := service.Delete(created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(service.List()) != 0 {
		t.Fatal("expected snippet to be deleted")
	}
}

func TestServiceRejectsDuplicateNames(t *testing.T) {
	service, err := NewService(&memoryRepository{})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if _, err := service.Create(snippetdomain.Snippet{Name: "Deploy", Body: "one"}); err != nil {
		t.Fatalf("create first: %v", err)
	}
	if _, err := service.Create(snippetdomain.Snippet{Name: "deploy", Body: "two"}); err == nil {
		t.Fatal("expected duplicate name rejection")
	}
}
