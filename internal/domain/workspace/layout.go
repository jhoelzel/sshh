package workspace

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	MaxTabs           = 64
	maxIDLength       = 128
	maxNameLength     = 120
	maxTitleLength    = 255
	maxEndpointLength = 512
	MinSplitRatio     = 0.2
	MaxSplitRatio     = 0.8
)

type SplitAxis string

const (
	SplitAxisRow    SplitAxis = "row"
	SplitAxisColumn SplitAxis = "column"
)

type Tab struct {
	ProfileID string `json:"profileId"`
	Title     string `json:"title"`
	Endpoint  string `json:"endpoint"`
}

type Split struct {
	Axis         SplitAxis `json:"axis"`
	PrimaryTab   int       `json:"primaryTab"`
	SecondaryTab int       `json:"secondaryTab"`
	Ratio        float64   `json:"ratio"`
}

type Layout struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Tabs      []Tab     `json:"tabs"`
	ActiveTab int       `json:"activeTab"`
	Split     *Split    `json:"split,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (layout Layout) WithDefaults(now time.Time) Layout {
	layout.ID = strings.TrimSpace(layout.ID)
	layout.Name = strings.TrimSpace(layout.Name)
	layout.Tabs = append([]Tab(nil), layout.Tabs...)
	for index := range layout.Tabs {
		layout.Tabs[index].ProfileID = strings.TrimSpace(layout.Tabs[index].ProfileID)
		layout.Tabs[index].Title = strings.TrimSpace(layout.Tabs[index].Title)
		layout.Tabs[index].Endpoint = strings.TrimSpace(layout.Tabs[index].Endpoint)
	}
	if layout.Split != nil {
		split := *layout.Split
		split.Axis = SplitAxis(strings.TrimSpace(string(split.Axis)))
		layout.Split = &split
	}
	if layout.CreatedAt.IsZero() {
		layout.CreatedAt = now.UTC()
	}
	if layout.UpdatedAt.IsZero() {
		layout.UpdatedAt = layout.CreatedAt
	}
	return layout
}

func (layout Layout) Validate() error {
	if err := validateText("id", layout.ID, maxIDLength, false); err != nil {
		return err
	}
	if err := validateText("name", layout.Name, maxNameLength, false); err != nil {
		return err
	}
	if len(layout.Tabs) == 0 {
		return errors.New("workspace layout requires at least one tab")
	}
	if len(layout.Tabs) > MaxTabs {
		return fmt.Errorf("workspace layout has more than %d tabs", MaxTabs)
	}
	if layout.ActiveTab < 0 || layout.ActiveTab >= len(layout.Tabs) {
		return errors.New("workspace layout active tab is out of range")
	}
	for index, tab := range layout.Tabs {
		if err := validateText("tab profile id", tab.ProfileID, maxIDLength, false); err != nil {
			return fmt.Errorf("workspace layout tab %d: %w", index+1, err)
		}
		if err := validateText("tab title", tab.Title, maxTitleLength, false); err != nil {
			return fmt.Errorf("workspace layout tab %d: %w", index+1, err)
		}
		if err := validateText("tab endpoint", tab.Endpoint, maxEndpointLength, true); err != nil {
			return fmt.Errorf("workspace layout tab %d: %w", index+1, err)
		}
	}
	if layout.Split != nil {
		if layout.Split.Axis != SplitAxisRow && layout.Split.Axis != SplitAxisColumn {
			return errors.New("workspace layout split axis is invalid")
		}
		if layout.Split.PrimaryTab < 0 || layout.Split.PrimaryTab >= len(layout.Tabs) ||
			layout.Split.SecondaryTab < 0 || layout.Split.SecondaryTab >= len(layout.Tabs) {
			return errors.New("workspace layout split tab is out of range")
		}
		if layout.Split.PrimaryTab == layout.Split.SecondaryTab {
			return errors.New("workspace layout split tabs must be distinct")
		}
		if layout.ActiveTab != layout.Split.PrimaryTab && layout.ActiveTab != layout.Split.SecondaryTab {
			return errors.New("workspace layout active tab is not visible in the split")
		}
		if math.IsNaN(layout.Split.Ratio) || math.IsInf(layout.Split.Ratio, 0) ||
			layout.Split.Ratio < MinSplitRatio || layout.Split.Ratio > MaxSplitRatio {
			return fmt.Errorf("workspace layout split ratio must be between %.1f and %.1f", MinSplitRatio, MaxSplitRatio)
		}
	}
	return nil
}

func validateText(field, value string, limit int, optional bool) error {
	if value == "" {
		if optional {
			return nil
		}
		return fmt.Errorf("workspace layout %s is required", field)
	}
	if len(value) > limit {
		return fmt.Errorf("workspace layout %s exceeds %d bytes", field, limit)
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return fmt.Errorf("workspace layout %s contains a control character", field)
		}
	}
	return nil
}
