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

func TestLayoutValidatesSplitGeometryAndVisibleActiveTab(t *testing.T) {
	layout := Layout{
		ID: "layout-1", Name: "Operations", ActiveTab: 1,
		Tabs:  []Tab{{ProfileID: "profile-1", Title: "Production"}, {ProfileID: "profile-2", Title: "Database"}},
		Split: &Split{Axis: SplitAxisRow, PrimaryTab: 0, SecondaryTab: 1, Ratio: 0.5},
	}
	if err := layout.Validate(); err != nil {
		t.Fatalf("validate split layout: %v", err)
	}

	tests := []struct {
		name   string
		split  Split
		active int
		want   string
	}{
		{name: "axis", split: Split{Axis: "diagonal", PrimaryTab: 0, SecondaryTab: 1, Ratio: 0.5}, active: 0, want: "axis"},
		{name: "range", split: Split{Axis: SplitAxisRow, PrimaryTab: 0, SecondaryTab: 3, Ratio: 0.5}, active: 0, want: "out of range"},
		{name: "duplicate", split: Split{Axis: SplitAxisRow, PrimaryTab: 0, SecondaryTab: 0, Ratio: 0.5}, active: 0, want: "distinct"},
		{name: "active", split: Split{Axis: SplitAxisRow, PrimaryTab: 0, SecondaryTab: 1, Ratio: 0.5}, active: 2, want: "active tab"},
		{name: "ratio", split: Split{Axis: SplitAxisColumn, PrimaryTab: 0, SecondaryTab: 1, Ratio: 0.9}, active: 0, want: "ratio"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := layout
			candidate.Tabs = append(candidate.Tabs, Tab{ProfileID: "profile-3", Title: "Logs"})
			candidate.Split = &test.split
			candidate.ActiveTab = test.active
			if err := candidate.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected %q split error, got %v", test.want, err)
			}
		})
	}
}
