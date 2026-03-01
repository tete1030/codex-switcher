package app

import (
	"encoding/json"
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

func TestSwitchSameActiveProfileIsIdempotentWithoutLastSnapshot(t *testing.T) {
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
			"account_id":    "acct-active",
		},
	}); err != nil {
		t.Fatalf("write active auth: %v", err)
	}

	if err := saveProfile(paths, "my", Credential{
		Provider:  "openai-codex",
		Access:    "stale-access",
		Refresh:   "stale-refresh",
		AccountID: "acct-stale",
	}, true); err != nil {
		t.Fatalf("save stale my profile: %v", err)
	}

	if err := saveState(paths, StateFile{
		Version:         1,
		ActiveProfile:   "my",
		PreviousProfile: "buy1",
	}); err != nil {
		t.Fatalf("save state: %v", err)
	}

	svc := NewService()
	results, err := svc.Switch("my", []ToolName{ToolCodex}, SwitchOptions{})
	if err != nil {
		t.Fatalf("switch failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one switch result, got %d", len(results))
	}
	if results[0].Status != "already_active" {
		t.Fatalf("expected status already_active, got %q", results[0].Status)
	}
	if results[0].Changed {
		t.Fatalf("expected Changed=false for already_active switch")
	}
	if results[0].SnapshotProfile != "" {
		t.Fatalf("expected no snapshot profile, got %q", results[0].SnapshotProfile)
	}

	if _, err := os.Stat(profilePath(paths, "__last__")); !os.IsNotExist(err) {
		t.Fatalf("expected __last__ not created, got err=%v", err)
	}

	activeTokens, err := readCodexTokens(paths.ActivePath)
	if err != nil {
		t.Fatalf("read active tokens: %v", err)
	}
	if activeTokens["access_token"] != "active-access" || activeTokens["refresh_token"] != "active-refresh" {
		t.Fatalf("expected active auth unchanged, got %+v", activeTokens)
	}

	profile, err := loadProfile(paths, "my")
	if err != nil {
		t.Fatalf("load my profile: %v", err)
	}
	if profile.Access != "active-access" || profile.Refresh != "active-refresh" || profile.AccountID != "acct-active" {
		t.Fatalf("expected my profile synced from active auth, got %+v", profile)
	}

	state, err := loadState(paths)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if state.ActiveProfile != "my" {
		t.Fatalf("expected active profile my, got %q", state.ActiveProfile)
	}
	if state.PreviousProfile != "buy1" {
		t.Fatalf("expected previous profile preserved as buy1, got %q", state.PreviousProfile)
	}
}

func TestSwitchSameActiveProfileRepairsMissingActiveWithoutSnapshot(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CODEX_HOME", filepath.Join(tmp, "codex-home"))

	paths, err := resolveToolPaths(ToolCodex)
	if err != nil {
		t.Fatalf("resolve paths: %v", err)
	}

	if err := saveProfile(paths, "my", Credential{
		Provider:  "openai-codex",
		Access:    "profile-access",
		Refresh:   "profile-refresh",
		AccountID: "acct-profile",
	}, true); err != nil {
		t.Fatalf("save profile: %v", err)
	}

	if err := saveState(paths, StateFile{
		Version:         1,
		ActiveProfile:   "my",
		PreviousProfile: "buy1",
	}); err != nil {
		t.Fatalf("save state: %v", err)
	}

	svc := NewService()
	results, err := svc.Switch("my", []ToolName{ToolCodex}, SwitchOptions{})
	if err != nil {
		t.Fatalf("switch failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one switch result, got %d", len(results))
	}
	if results[0].Status != "switched" {
		t.Fatalf("expected switched status when repairing missing active creds, got %q", results[0].Status)
	}
	if !results[0].Changed {
		t.Fatalf("expected Changed=true for repaired active creds")
	}
	if results[0].SnapshotProfile != "" {
		t.Fatalf("expected no snapshot profile when same active target, got %q", results[0].SnapshotProfile)
	}

	if _, err := os.Stat(profilePath(paths, "__last__")); !os.IsNotExist(err) {
		t.Fatalf("expected __last__ not created, got err=%v", err)
	}

	activeTokens, err := readCodexTokens(paths.ActivePath)
	if err != nil {
		t.Fatalf("read active tokens: %v", err)
	}
	if activeTokens["access_token"] != "profile-access" || activeTokens["refresh_token"] != "profile-refresh" {
		t.Fatalf("expected active auth repaired from profile, got %+v", activeTokens)
	}
}

func readCodexTokens(path string) (map[string]string, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var root map[string]any
	if err := json.Unmarshal(bytes, &root); err != nil {
		return nil, err
	}
	raw, _ := root["tokens"].(map[string]any)
	out := map[string]string{}
	for _, key := range []string{"access_token", "refresh_token", "account_id"} {
		if v, ok := raw[key].(string); ok {
			out[key] = v
		}
	}
	return out, nil
}
