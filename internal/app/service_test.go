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
	if _, ok := updatedStore.Profiles[targetID]; !ok {
		t.Fatalf("expected openclaw auth store untouched during profile delete")
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
	if _, ok := store.Profiles[openClawManagedProfileID]; !ok {
		t.Fatalf("expected openclaw managed profile in active store")
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

func TestSwitchOpenClawStaleStateRewritesActiveCredential(t *testing.T) {
	tmp := t.TempDir()
	agentDir := filepath.Join(tmp, "agent")
	t.Setenv("OPENCLAW_AGENT_DIR", agentDir)

	paths, err := resolveToolPaths(ToolOpenClaw)
	if err != nil {
		t.Fatalf("resolve paths: %v", err)
	}

	if err := saveProfile(paths, "my", Credential{Provider: "openai-codex", Access: "my-access", Refresh: "my-refresh", AccountID: "acct-my"}, true); err != nil {
		t.Fatalf("save my profile: %v", err)
	}
	if err := saveProfile(paths, "buy1", Credential{Provider: "openai-codex", Access: "buy1-access", Refresh: "buy1-refresh", AccountID: "acct-buy1"}, true); err != nil {
		t.Fatalf("save buy1 profile: %v", err)
	}
	if err := writeJSONAtomic(paths.ActivePath, map[string]any{
		"version": 1,
		"profiles": map[string]any{
			openClawManagedProfileID: map[string]any{
				"type":      "oauth",
				"provider":  "openai-codex",
				"access":    "buy1-access",
				"refresh":   "buy1-refresh",
				"accountId": "acct-buy1",
			},
		},
		"order": map[string]any{
			"openai-codex": []string{openClawManagedProfileID},
		},
	}); err != nil {
		t.Fatalf("write openclaw active store: %v", err)
	}
	if err := saveState(paths, StateFile{Version: 1, ActiveProfile: "my", PreviousProfile: "buy1"}); err != nil {
		t.Fatalf("save state: %v", err)
	}

	svc := NewService()
	results, err := svc.Switch("my", []ToolName{ToolOpenClaw}, SwitchOptions{})
	if err != nil {
		t.Fatalf("switch failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one switch result, got %d", len(results))
	}
	if results[0].Status != "switched" {
		t.Fatalf("expected switched status for stale-state openclaw switch, got %q", results[0].Status)
	}
	if !results[0].Changed {
		t.Fatalf("expected changed=true when stale-state openclaw switch rewrites active auth")
	}

	store, err := readOpenClawStore(paths.ActivePath)
	if err != nil {
		t.Fatalf("read openclaw store: %v", err)
	}
	managed := store.Profiles[openClawManagedProfileID]
	if managed.Access != "my-access" || managed.Refresh != "my-refresh" || managed.AccountID != "acct-my" {
		t.Fatalf("expected openclaw active store rewritten to my profile, got %+v", managed)
	}

	updatedState, err := loadState(paths)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if updatedState.ActiveProfile != "my" {
		t.Fatalf("expected active profile my, got %q", updatedState.ActiveProfile)
	}
	if updatedState.ActiveCredentialHash == "" {
		t.Fatalf("expected active credential hash recorded after switch")
	}
}

func TestRenameProfileRenamesNonActiveProfile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CODEX_HOME", filepath.Join(tmp, "codex-home"))

	paths, err := resolveToolPaths(ToolCodex)
	if err != nil {
		t.Fatalf("resolve paths: %v", err)
	}

	if err := saveProfile(paths, "buy1", Credential{Provider: "openai-codex", Access: "a", Refresh: "r", AccountID: "acct-1"}, true); err != nil {
		t.Fatalf("save profile: %v", err)
	}
	if err := saveState(paths, StateFile{Version: 1, ActiveProfile: "my", PreviousProfile: "last", PendingCreateProfile: "pending", PendingCreateSince: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatalf("save state: %v", err)
	}

	svc := NewService()
	results, err := svc.RenameProfile("buy1", "buy2", []ToolName{ToolCodex})
	if err != nil {
		t.Fatalf("rename profile failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one result, got %d", len(results))
	}
	if results[0].FromProfile != "buy1" || results[0].ToProfile != "buy2" || !results[0].Changed {
		t.Fatalf("unexpected rename result: %+v", results[0])
	}

	if _, err := os.Stat(profilePath(paths, "buy1")); !os.IsNotExist(err) {
		t.Fatalf("expected old profile removed, got err=%v", err)
	}
	renamed, err := loadProfile(paths, "buy2")
	if err != nil {
		t.Fatalf("expected renamed profile to exist: %v", err)
	}
	if renamed.AccountID != "acct-1" {
		t.Fatalf("expected account id preserved, got %q", renamed.AccountID)
	}

	state, err := loadState(paths)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if state.ActiveProfile != "my" || state.PreviousProfile != "last" || state.PendingCreateProfile != "pending" {
		t.Fatalf("expected unrelated state unchanged, got %+v", state)
	}
}

func TestRenameProfileUpdatesStateReferencesWhenActive(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CODEX_HOME", filepath.Join(tmp, "codex-home"))

	paths, err := resolveToolPaths(ToolCodex)
	if err != nil {
		t.Fatalf("resolve paths: %v", err)
	}

	if err := saveProfile(paths, "my", Credential{Provider: "openai-codex", Access: "a", Refresh: "r"}, true); err != nil {
		t.Fatalf("save profile: %v", err)
	}
	if err := saveState(paths, StateFile{Version: 1, ActiveProfile: "my", PreviousProfile: "my", PendingCreateProfile: "my", PendingCreateSince: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatalf("save state: %v", err)
	}

	svc := NewService()
	if _, err := svc.RenameProfile("my", "my2", []ToolName{ToolCodex}); err != nil {
		t.Fatalf("rename profile failed: %v", err)
	}

	state, err := loadState(paths)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if state.ActiveProfile != "my2" || state.PreviousProfile != "my2" || state.PendingCreateProfile != "my2" {
		t.Fatalf("expected state references updated, got %+v", state)
	}
	if state.PendingCreateSince != "2026-01-01T00:00:00Z" {
		t.Fatalf("expected pending_create_since unchanged, got %q", state.PendingCreateSince)
	}
}

func TestRenameProfileFailsWhenSourceMissingInStrictMode(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CODEX_HOME", filepath.Join(tmp, "codex-home"))

	svc := NewService()
	_, err := svc.RenameProfile("missing", "new", []ToolName{ToolCodex})
	if err == nil {
		t.Fatalf("expected error for missing source profile")
	}
	if ExitCode(err) != ExitUserError {
		t.Fatalf("expected exit code %d, got %d (err=%v)", ExitUserError, ExitCode(err), err)
	}
}

func TestRenameProfileFailsWhenTargetExistsInStrictMode(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CODEX_HOME", filepath.Join(tmp, "codex-home"))

	paths, err := resolveToolPaths(ToolCodex)
	if err != nil {
		t.Fatalf("resolve paths: %v", err)
	}

	if err := saveProfile(paths, "old", Credential{Provider: "openai-codex", Access: "a", Refresh: "r"}, true); err != nil {
		t.Fatalf("save old profile: %v", err)
	}
	if err := saveProfile(paths, "new", Credential{Provider: "openai-codex", Access: "x", Refresh: "y"}, true); err != nil {
		t.Fatalf("save new profile: %v", err)
	}

	svc := NewService()
	_, err = svc.RenameProfile("old", "new", []ToolName{ToolCodex})
	if err == nil {
		t.Fatalf("expected error when target profile exists")
	}
	if ExitCode(err) != ExitUserError {
		t.Fatalf("expected exit code %d, got %d (err=%v)", ExitUserError, ExitCode(err), err)
	}

	if _, err := loadProfile(paths, "old"); err != nil {
		t.Fatalf("expected old profile unchanged, got err=%v", err)
	}
}

func TestRenameProfileOpenClawUpdatesStateAndLeavesStoreUntouched(t *testing.T) {
	tmp := t.TempDir()
	agentDir := filepath.Join(tmp, "agent")
	t.Setenv("OPENCLAW_AGENT_DIR", agentDir)

	paths, err := resolveToolPaths(ToolOpenClaw)
	if err != nil {
		t.Fatalf("resolve paths: %v", err)
	}

	if err := saveProfile(paths, "buy1", Credential{Provider: "openai-codex", Access: "a", Refresh: "r"}, true); err != nil {
		t.Fatalf("save profile: %v", err)
	}
	if err := saveState(paths, StateFile{Version: 1, ActiveProfile: "buy1", PreviousProfile: "buy1", PendingCreateProfile: "buy1", PendingCreateSince: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatalf("save state: %v", err)
	}

	store := map[string]any{
		"version": 1,
		"profiles": map[string]any{
			"openai-codex:default": map[string]any{
				"type":      "oauth",
				"provider":  "openai-codex",
				"access":    "a",
				"refresh":   "r",
				"accountId": "acct-1",
			},
			"openai-codex:manual": map[string]any{
				"type":      "oauth",
				"provider":  "openai-codex",
				"access":    "x",
				"refresh":   "y",
				"accountId": "acct-2",
			},
		},
		"order": map[string]any{
			"openai-codex": []string{"openai-codex:default", "openai-codex:manual"},
		},
	}
	if err := writeJSONAtomic(paths.ActivePath, store); err != nil {
		t.Fatalf("write openclaw store: %v", err)
	}

	originalStore, err := readOpenClawStore(paths.ActivePath)
	if err != nil {
		t.Fatalf("read original openclaw store: %v", err)
	}

	svc := NewService()
	if _, err := svc.RenameProfile("buy1", "buy2", []ToolName{ToolOpenClaw}); err != nil {
		t.Fatalf("rename profile failed: %v", err)
	}

	updatedStore, err := readOpenClawStore(paths.ActivePath)
	if err != nil {
		t.Fatalf("read openclaw store: %v", err)
	}
	if len(updatedStore.Profiles) != len(originalStore.Profiles) {
		t.Fatalf("expected openclaw profiles unchanged, before=%d after=%d", len(originalStore.Profiles), len(updatedStore.Profiles))
	}
	if len(updatedStore.Order["openai-codex"]) != len(originalStore.Order["openai-codex"]) {
		t.Fatalf("expected openclaw order unchanged, before=%+v after=%+v", originalStore.Order["openai-codex"], updatedStore.Order["openai-codex"])
	}
	if updatedStore.Order["openai-codex"][0] != "openai-codex:default" {
		t.Fatalf("expected active order to remain default, got %+v", updatedStore.Order["openai-codex"])
	}

	state, err := loadState(paths)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if state.ActiveProfile != "buy2" || state.PreviousProfile != "buy2" || state.PendingCreateProfile != "buy2" {
		t.Fatalf("expected state references renamed for openclaw, got %+v", state)
	}
}

func TestMigrateOpenClawMigratesLegacyRotaterStore(t *testing.T) {
	tmp := t.TempDir()
	agentDir := filepath.Join(tmp, "agent")
	t.Setenv("OPENCLAW_AGENT_DIR", agentDir)

	paths, err := resolveToolPaths(ToolOpenClaw)
	if err != nil {
		t.Fatalf("resolve paths: %v", err)
	}

	store := map[string]any{
		"version": 1,
		"profiles": map[string]any{
			"openai-codex:rotater:buy1": map[string]any{
				"type":      "oauth",
				"provider":  "openai-codex",
				"access":    "legacy-access",
				"refresh":   "legacy-refresh",
				"accountId": "acct-legacy",
			},
			"openai-codex:manual": map[string]any{
				"type":      "oauth",
				"provider":  "openai-codex",
				"access":    "manual-access",
				"refresh":   "manual-refresh",
				"accountId": "acct-manual",
			},
		},
		"order": map[string]any{
			"openai-codex": []string{"openai-codex:rotater:buy1", openClawLegacyPendingLoginSentinelID, "openai-codex:manual"},
		},
		openClawLegacyPendingKnownIDsKey: []string{"openai-codex:rotater:buy1", "openai-codex:manual"},
	}
	if err := writeJSONAtomic(paths.ActivePath, store); err != nil {
		t.Fatalf("write legacy store: %v", err)
	}

	svc := NewService()
	result, err := svc.MigrateOpenClaw()
	if err != nil {
		t.Fatalf("migrate openclaw failed: %v", err)
	}
	if result.Status != "migrated" || !result.Changed {
		t.Fatalf("expected migrated result, got %+v", result)
	}
	if result.RemovedLegacyRotater != 1 {
		t.Fatalf("expected one legacy rotater removed, got %d", result.RemovedLegacyRotater)
	}
	if !result.RemovedPendingSentinel || !result.RemovedPendingKnownState {
		t.Fatalf("expected legacy pending markers removed, got %+v", result)
	}
	if !result.ManagedProfileSet {
		t.Fatalf("expected managed profile set, got %+v", result)
	}

	updated, err := readOpenClawStore(paths.ActivePath)
	if err != nil {
		t.Fatalf("read migrated store: %v", err)
	}
	if _, ok := updated.Profiles["openai-codex:rotater:buy1"]; ok {
		t.Fatalf("expected legacy rotater profile removed")
	}
	managed, ok := updated.Profiles[openClawManagedProfileID]
	if !ok {
		t.Fatalf("expected managed profile present")
	}
	if managed.Access != "legacy-access" || managed.Refresh != "legacy-refresh" || managed.AccountID != "acct-legacy" {
		t.Fatalf("expected managed profile from active legacy credential, got %+v", managed)
	}
	order := updated.Order["openai-codex"]
	if len(order) != 1 || order[0] != openClawManagedProfileID {
		t.Fatalf("expected single managed active order, got %+v", order)
	}
	if _, ok := updated.Raw[openClawLegacyPendingKnownIDsKey]; ok {
		t.Fatalf("expected pending known marker removed")
	}
}

func TestMigrateOpenClawNoStore(t *testing.T) {
	tmp := t.TempDir()
	agentDir := filepath.Join(tmp, "agent")
	t.Setenv("OPENCLAW_AGENT_DIR", agentDir)

	svc := NewService()
	result, err := svc.MigrateOpenClaw()
	if err != nil {
		t.Fatalf("migrate openclaw failed: %v", err)
	}
	if result.Status != "no_store" || result.Changed {
		t.Fatalf("expected no_store with no changes, got %+v", result)
	}
}

func TestMigrateOpenClawNoChanges(t *testing.T) {
	tmp := t.TempDir()
	agentDir := filepath.Join(tmp, "agent")
	t.Setenv("OPENCLAW_AGENT_DIR", agentDir)

	paths, err := resolveToolPaths(ToolOpenClaw)
	if err != nil {
		t.Fatalf("resolve paths: %v", err)
	}

	store := map[string]any{
		"version": 1,
		"profiles": map[string]any{
			openClawManagedProfileID: map[string]any{
				"type":      "oauth",
				"provider":  "openai-codex",
				"access":    "managed-access",
				"refresh":   "managed-refresh",
				"accountId": "acct-managed",
				"clientId":  "app_EMoamEEZ73f0CkXaXp7hrann",
			},
		},
		"order": map[string]any{
			"openai-codex": []string{openClawManagedProfileID},
		},
	}
	if err := writeJSONAtomic(paths.ActivePath, store); err != nil {
		t.Fatalf("write store: %v", err)
	}

	svc := NewService()
	result, err := svc.MigrateOpenClaw()
	if err != nil {
		t.Fatalf("migrate openclaw failed: %v", err)
	}
	if result.Status != "no_changes" || result.Changed {
		t.Fatalf("expected no_changes, got %+v", result)
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
