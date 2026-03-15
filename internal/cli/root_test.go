package cli

import (
	"testing"
	"time"

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

func TestValidateUsageWatchOptionsRequiresTools(t *testing.T) {
	err := validateUsageWatchOptions(true, 15*time.Second, nil, "", false, false)
	if err == nil || err.Error() != "--watch requires --tools" {
		t.Fatalf("expected requires-tools error, got %v", err)
	}
}

func TestValidateUsageWatchOptionsRejectsProfile(t *testing.T) {
	err := validateUsageWatchOptions(true, 15*time.Second, []app.ToolName{app.ToolCodex}, "my", false, false)
	if err == nil || err.Error() != "--watch cannot be combined with --profile" {
		t.Fatalf("expected profile conflict error, got %v", err)
	}
}

func TestValidateUsageWatchOptionsRejectsAllProfiles(t *testing.T) {
	err := validateUsageWatchOptions(true, 15*time.Second, []app.ToolName{app.ToolCodex}, "", true, false)
	if err == nil || err.Error() != "--watch cannot be combined with --all-profiles" {
		t.Fatalf("expected all-profiles conflict error, got %v", err)
	}
}

func TestValidateUsageWatchOptionsRejectsJSON(t *testing.T) {
	err := validateUsageWatchOptions(true, 15*time.Second, []app.ToolName{app.ToolCodex}, "", false, true)
	if err == nil || err.Error() != "--watch cannot be combined with --json" {
		t.Fatalf("expected json conflict error, got %v", err)
	}
}

func TestValidateUsageWatchOptionsRejectsNonPositiveInterval(t *testing.T) {
	err := validateUsageWatchOptions(true, 0, []app.ToolName{app.ToolCodex}, "", false, false)
	if err == nil || err.Error() != "--interval must be greater than 0" {
		t.Fatalf("expected interval error, got %v", err)
	}
}

func TestValidateUsageWatchOptionsAcceptsSelectedTools(t *testing.T) {
	err := validateUsageWatchOptions(true, 15*time.Second, []app.ToolName{app.ToolCodex, app.ToolOpenClaw}, "", false, false)
	if err != nil {
		t.Fatalf("expected watch options to be accepted, got %v", err)
	}
}
