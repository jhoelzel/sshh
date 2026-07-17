package workspace

import (
	"testing"

	workspacedomain "shh-h/internal/domain/workspace"
)

type memoryRepository struct {
	layouts []workspacedomain.Layout
}

func (repository *memoryRepository) LoadLayouts() ([]workspacedomain.Layout, error) {
	return clone(repository.layouts), nil
}

func (repository *memoryRepository) SaveLayouts(layouts []workspacedomain.Layout) error {
	repository.layouts = clone(layouts)
	return nil
}

func TestServiceCRUD(t *testing.T) {
	service, err := NewService(&memoryRepository{})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	created, err := service.Create(workspacedomain.Layout{
		Name: "Operations",
		Tabs: []workspacedomain.Tab{{ProfileID: "profile-1", Title: "Production", Endpoint: "prod.example:22"}},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	created.Name = "Production"
	created.Tabs = append(created.Tabs, workspacedomain.Tab{ProfileID: "profile-2", Title: "Database"})
	created.ActiveTab = 1
	updated, err := service.Update(created)
	if err != nil || updated.Name != "Production" || len(updated.Tabs) != 2 || updated.ActiveTab != 1 {
		t.Fatalf("update: layout=%#v err=%v", updated, err)
	}
	if err := service.Delete(created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(service.List()) != 0 {
		t.Fatal("expected layout to be deleted")
	}
}

func TestServiceRejectsDuplicateNames(t *testing.T) {
	service, err := NewService(&memoryRepository{})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	candidate := workspacedomain.Layout{Name: "Operations", Tabs: []workspacedomain.Tab{{ProfileID: "profile-1", Title: "Production"}}}
	if _, err := service.Create(candidate); err != nil {
		t.Fatalf("create first: %v", err)
	}
	candidate.Name = "operations"
	if _, err := service.Create(candidate); err == nil {
		t.Fatal("expected duplicate name rejection")
	}
}

func TestServiceReturnsIndependentCopies(t *testing.T) {
	service, err := NewService(&memoryRepository{})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	created, err := service.Create(workspacedomain.Layout{Name: "Operations", Tabs: []workspacedomain.Tab{{ProfileID: "profile-1", Title: "Production"}}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	created.Tabs[0].Title = "Changed"
	listed := service.List()
	if listed[0].Tabs[0].Title != "Production" {
		t.Fatal("mutating returned layout changed service state")
	}
}
