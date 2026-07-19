package settings

import (
	"math"
	"testing"
)

func TestDefaultsAreValid(t *testing.T) {
	defaults := Defaults()
	if err := defaults.Validate(); err != nil {
		t.Fatalf("default settings are invalid: %v", err)
	}
}

func TestSettingsRejectOutOfRangeTerminalValues(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Settings)
	}{
		{name: "font", change: func(value *Settings) { value.Terminal.FontFamily = "unknown" }},
		{name: "size", change: func(value *Settings) { value.Terminal.FontSize = 9 }},
		{name: "height", change: func(value *Settings) { value.Terminal.LineHeight = 2.1 }},
		{name: "nan height", change: func(value *Settings) { value.Terminal.LineHeight = math.NaN() }},
		{name: "cursor", change: func(value *Settings) { value.Terminal.CursorStyle = "box" }},
		{name: "scrollback", change: func(value *Settings) { value.Terminal.Scrollback = 100 }},
		{name: "connect timeout", change: func(value *Settings) { value.Connection.ConnectTimeoutSeconds = 2 }},
		{name: "keepalive interval", change: func(value *Settings) { value.Connection.KeepAliveIntervalSeconds = 4 }},
		{name: "keepalive failures", change: func(value *Settings) { value.Connection.KeepAliveMaxFailures = 0 }},
		{name: "transfer notification threshold", change: func(value *Settings) { value.Notifications.LongTransferSeconds = 4 }},
		{name: "transfer concurrency", change: func(value *Settings) { value.Transfers.Concurrency = 0 }},
		{name: "collision policy", change: func(value *Settings) { value.Transfers.CollisionPolicy = "replace-maybe" }},
		{name: "theme", change: func(value *Settings) { value.UI.Theme = "sepia" }},
		{name: "sidebar width", change: func(value *Settings) { value.UI.SidebarWidth = MinSidebarWidth - 1 }},
		{name: "workspace", change: func(value *Settings) { value.UI.Workspace = "connections" }},
		{name: "window width", change: func(value *Settings) { value.Window.Width = MinWindowWidth - 1 }},
		{name: "window height", change: func(value *Settings) { value.Window.Height = MaxWindowSize + 1 }},
		{name: "window x", change: func(value *Settings) { value.Window.X = MaxWindowOffset + 1; value.Window.Positioned = true }},
		{name: "window y", change: func(value *Settings) { value.Window.Y = -MaxWindowOffset - 1; value.Window.Positioned = true }},
		{name: "position flag", change: func(value *Settings) { value.Window.X = 20 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := Defaults()
			test.change(&value)
			if err := value.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
