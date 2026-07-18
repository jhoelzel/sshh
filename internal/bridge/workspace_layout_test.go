package bridge

import (
	"testing"

	workspacedomain "shh-h/internal/domain/workspace"
)

func TestWorkspaceLayoutSplitDTOConversion(t *testing.T) {
	input := WorkspaceLayoutInputDTO{
		ID: "layout-1", Name: "Operations", ActiveTab: 1,
		Tabs:  []WorkspaceTabDTO{{ProfileID: "profile-1", Title: "Production"}, {ProfileID: "profile-2", Title: "Database"}},
		Split: &WorkspaceSplitDTO{Axis: "column", PrimaryTab: 0, SecondaryTab: 1, Ratio: 0.6},
	}
	domain := workspaceLayoutFromInput(input)
	if domain.Split == nil || domain.Split.Axis != workspacedomain.SplitAxisColumn || domain.Split.Ratio != 0.6 {
		t.Fatalf("unexpected domain split: %#v", domain.Split)
	}

	dto := workspaceLayoutDTO(domain)
	if dto.Split == nil || dto.Split.Axis != "column" || dto.Split.SecondaryTab != 1 || dto.Split.Ratio != 0.6 {
		t.Fatalf("unexpected split DTO: %#v", dto.Split)
	}
}
