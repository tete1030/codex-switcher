package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseToolsDefault(t *testing.T) {
	tools, err := ParseTools("")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(tools) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(tools))
	}
}

func TestParseToolsInvalid(t *testing.T) {
	_, err := ParseTools("codex,unknown")
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestDeleteProfileRemovesFileAndStateReferences(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CODEX_HOME", filepath.Join(tmp, "codex-home"))

	paths, err := resolveToolPaths(ToolCodex)
	if err != nil {
		t.Fatalf("resolve paths: %v", err)
	}

	if err := saveProfile(paths, "__last__", Credential{Provider: "openai-codex", Access: "a", Refresh: "r"}, true); err != nil {
		t.Fatalf("save profile: %v", err)
	}

	state := StateFile{
		Version:              1,
		ActiveProfile:        "__last__",
		PreviousProfile:      "__last__",
		PendingCreateProfile: "__last__",
		PendingCreateSince:   "2026-01-01T00:00:00Z",
	}
	if err := saveState(paths, state); err != nil {
		t.Fatalf("save state: %v", err)
	}

	svc := NewService()
	if err := svc.DeleteProfile("__last__", []ToolName{ToolCodex}); err != nil {
		t.Fatalf("delete profile: %v", err)
	}

	if _, err := os.Stat(profilePath(paths, "__last__")); !os.IsNotExist(err) {
		t.Fatalf("expected profile file removed, got err=%v", err)
	}

	updated, err := loadState(paths)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if updated.ActiveProfile != "" || updated.PreviousProfile != "" || updated.PendingCreateProfile != "" || updated.PendingCreateSince != "" {
		t.Fatalf("expected state references cleared, got %+v", updated)
	}
}

func TestDeleteProfileRemovesOpenClawRotaterStoreEntry(t *testing.T) {
	tmp := t.TempDir()
	agentDir := filepath.Join(tmp, "agent")
	t.Setenv("OPENCLAW_AGENT_DIR", agentDir)

	paths, err := resolveToolPaths(ToolOpenClaw)
	if err != nil {
		t.Fatalf("resolve paths: %v", err)
	}

	targetID := "openai-codex:rotater:__last__"
	store := map[string]any{
		"version": 1,
		"profiles": map[string]any{
			targetID: map[string]any{
				"type":      "oauth",
				"provider":  "openai-codex",
				"access":    "a",
				"refresh":   "r",
				"accountId": "acct",
			},
			"openai-codex:manual": map[string]any{
				"type":      "oauth",
				"provider":  "openai-codex",
				"access":    "x",
				"refresh":   "y",
				"accountId": "acct2",
			},
		},
		"order": map[string]any{
			"openai-codex": []string{targetID, "openai-codex:manual"},
		},
	}
	if err := writeJSONAtomic(paths.ActivePath, store); err != nil {
		t.Fatalf("write openclaw store: %v", err)
	}

	if err := saveProfile(paths, "__last__", Credential{Provider: "openai-codex", Access: "a", Refresh: "r"}, true); err != nil {
		t.Fatalf("save profile: %v", err)
	}
	if err := saveState(paths, StateFile{Version: 1, PreviousProfile: "__last__"}); err != nil {
		t.Fatalf("save state: %v", err)
	}

	svc := NewService()
	if err := svc.DeleteProfile("__last__", []ToolName{ToolOpenClaw}); err != nil {
		t.Fatalf("delete profile: %v", err)
	}

	if _, err := os.Stat(profilePath(paths, "__last__")); !os.IsNotExist(err) {
		t.Fatalf("expected profile file removed, got err=%v", err)
	}

	updatedStore, err := readOpenClawStore(paths.ActivePath)
	if err != nil {
		t.Fatalf("read openclaw store: %v", err)
	}
	if _, ok := updatedStore.Profiles[targetID]; ok {
		t.Fatalf("expected rotater profile removed from openclaw store")
	}
	order := updatedStore.Order["openai-codex"]
	for _, id := range order {
		if id == targetID {
			t.Fatalf("expected rotater profile removed from openclaw order")
		}
	}

	updatedState, err := loadState(paths)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if updatedState.PreviousProfile != "" {
		t.Fatalf("expected previous profile cleared, got %q", updatedState.PreviousProfile)
	}
}

func TestSwitchMaterializesPendingCreateProfileFromActiveCredential(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CODEX_HOME", filepath.Join(tmp, "codex-home"))

	paths, err := resolveToolPaths(ToolCodex)
	if err != nil {
		t.Fatalf("resolve paths: %v", err)
	}

	if err := writeJSONAtomic(paths.ActivePath, map[string]any{
		"auth_mode": "chatgpt",
		"tokens": map[string]any{
			"access_token":  "active-access",
			"refresh_token": "active-refresh",
			"account_id":    "acct-1",
		},
	}); err != nil {
		t.Fatalf("write active auth: %v", err)
	}

	if err := saveState(paths, StateFile{
		Version:              1,
		PendingCreateProfile: "buy1",
		PendingCreateSince:   "2026-01-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("save state: %v", err)
	}

	svc := NewService()
	results, err := svc.Switch("buy1", []ToolName{ToolCodex}, SwitchOptions{})
	if err != nil {
		t.Fatalf("switch failed: %v", err)
	}
	if len(results) != 1 || results[0].Status != "switched" {
		t.Fatalf("unexpected switch results: %+v", results)
	}

	materialized, err := loadProfile(paths, "buy1")
	if err != nil {
		t.Fatalf("expected buy1 profile materialized: %v", err)
	}
	if materialized.Access != "active-access" || materialized.Refresh != "active-refresh" || materialized.AccountID != "acct-1" {
		t.Fatalf("unexpected materialized profile %+v", materialized)
	}

	state, err := loadState(paths)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if state.ActiveProfile != "buy1" {
		t.Fatalf("expected active profile buy1, got %q", state.ActiveProfile)
	}
	if state.PendingCreateProfile != "" {
		t.Fatalf("expected pending create cleared, got %q", state.PendingCreateProfile)
	}
}

func TestSwitchMaterializesPendingCreateProfileFromOpenClawActiveCredential(t *testing.T) {
	tmp := t.TempDir()
	agentDir := filepath.Join(tmp, "agent")
	t.Setenv("OPENCLAW_AGENT_DIR", agentDir)

	paths, err := resolveToolPaths(ToolOpenClaw)
	if err != nil {
		t.Fatalf("resolve paths: %v", err)
	}

	activeStore := map[string]any{
		"version": 1,
		"profiles": map[string]any{
			"openai-codex:manual": map[string]any{
				"type":      "oauth",
				"provider":  "openai-codex",
				"access":    "active-access",
				"refresh":   "active-refresh",
				"accountId": "acct-1",
			},
		},
		"order": map[string]any{
			"openai-codex": []string{"openai-codex:manual"},
		},
	}
	if err := writeJSONAtomic(paths.ActivePath, activeStore); err != nil {
		t.Fatalf("write openclaw active store: %v", err)
	}

	if err := saveState(paths, StateFile{
		Version:              1,
		PendingCreateProfile: "buy1",
		PendingCreateSince:   "2026-01-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("save state: %v", err)
	}

	svc := NewService()
	results, err := svc.Switch("buy1", []ToolName{ToolOpenClaw}, SwitchOptions{})
	if err != nil {
		t.Fatalf("switch failed: %v", err)
	}
	if len(results) != 1 || results[0].Status != "switched" {
		t.Fatalf("unexpected switch results: %+v", results)
	}

	materialized, err := loadProfile(paths, "buy1")
	if err != nil {
		t.Fatalf("expected buy1 profile materialized: %v", err)
	}
	if materialized.Access != "active-access" || materialized.Refresh != "active-refresh" || materialized.AccountID != "acct-1" {
		t.Fatalf("unexpected materialized profile %+v", materialized)
	}

	store, err := readOpenClawStore(paths.ActivePath)
	if err != nil {
		t.Fatalf("read openclaw store: %v", err)
	}
	if _, ok := store.Profiles["openai-codex:rotater:buy1"]; !ok {
		t.Fatalf("expected openclaw rotater profile for buy1 in active store")
	}

	state, err := loadState(paths)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if state.ActiveProfile != "buy1" {
		t.Fatalf("expected active profile buy1, got %q", state.ActiveProfile)
	}
	if state.PendingCreateProfile != "" {
		t.Fatalf("expected pending create cleared, got %q", state.PendingCreateProfile)
	}
}
