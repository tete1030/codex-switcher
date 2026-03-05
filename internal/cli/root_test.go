package cli

import (
	"testing"

	"codex-switcher/internal/app"
)

func TestUsageDisplayLabelSingleResultNoActiveSuffix(t *testing.T) {
	item := app.UsageResult{Tool: app.ToolOpenClaw, Profile: "my"}
	active := map[app.ToolName]string{app.ToolOpenClaw: "my"}
	counts := map[app.ToolName]int{app.ToolOpenClaw: 1}

	label := usageDisplayLabel(item, active, counts)
	if label != "openclaw/my" {
		t.Fatalf("expected label without active suffix, got %q", label)
	}
}

func TestUsageDisplayLabelMultipleResultsShowsActiveSuffix(t *testing.T) {
	item := app.UsageResult{Tool: app.ToolOpenClaw, Profile: "my"}
	active := map[app.ToolName]string{app.ToolOpenClaw: "my"}
	counts := map[app.ToolName]int{app.ToolOpenClaw: 2}

	label := usageDisplayLabel(item, active, counts)
	if label != "openclaw/my (active)" {
		t.Fatalf("expected active suffix for multi-result tool, got %q", label)
	}
}

func TestUsageToolResultCounts(t *testing.T) {
	results := []app.UsageResult{
		{Tool: app.ToolCodex, Profile: "-"},
		{Tool: app.ToolOpenClaw, Profile: "my"},
		{Tool: app.ToolOpenClaw, Profile: "buy1"},
	}

	counts := usageToolResultCounts(results)
	if counts[app.ToolCodex] != 1 {
		t.Fatalf("expected codex count=1, got %d", counts[app.ToolCodex])
	}
	if counts[app.ToolOpenClaw] != 2 {
		t.Fatalf("expected openclaw count=2, got %d", counts[app.ToolOpenClaw])
	}
}
