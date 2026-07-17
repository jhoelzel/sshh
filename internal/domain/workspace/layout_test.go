package workspace

import (
	"strings"
	"testing"
	"time"
)

func TestLayoutWithDefaultsAndValidate(t *testing.T) {
	layout := Layout{
		ID:   " layout-1 ",
		Name: " Operations ",
		Tabs: []Tab{{ProfileID: " deleted-profile ", Title: " Production ", Endpoint: " host:22 "}},
	}.WithDefaults(time.Unix(100, 0))
	if err := layout.Validate(); err != nil {
		t.Fatalf("validate layout: %v", err)
	}
	if layout.ID != "layout-1" || layout.Name != "Operations" || layout.Tabs[0].ProfileID != "deleted-profile" {
		t.Fatalf("unexpected normalized layout: %#v", layout)
	}
	if layout.CreatedAt.IsZero() || layout.UpdatedAt.IsZero() {
		t.Fatal("expected timestamps to be populated")
	}
}

func TestLayoutRejectsInvalidActiveTab(t *testing.T) {
	layout := Layout{ID: "layout-1", Name: "Operations", Tabs: []Tab{{ProfileID: "profile-1", Title: "Production"}}, ActiveTab: 1}
	if err := layout.Validate(); err == nil || !strings.Contains(err.Error(), "active tab") {
		t.Fatalf("expected active tab validation error, got %v", err)
	}
}

func TestLayoutRejectsControlCharacters(t *testing.T) {
	layout := Layout{ID: "layout-1", Name: "Operations", Tabs: []Tab{{ProfileID: "profile-1", Title: "Production\n"}}}
	if err := layout.Validate(); err == nil || !strings.Contains(err.Error(), "control character") {
		t.Fatalf("expected control character validation error, got %v", err)
	}
}
