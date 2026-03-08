package app

import (
	"path/filepath"
	"testing"
)

func TestStatusOpenClawHidesStaleActiveProfile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", filepath.Join(tmp, "home"))
	t.Setenv("OPENCLAW_AGENT_DIR", filepath.Join(tmp, "agent"))

	paths, err := resolveToolPaths(ToolOpenClaw)
	if err != nil {
		t.Fatalf("resolve paths: %v", err)
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
	if err := saveProfile(paths, "my", Credential{Provider: "openai-codex", Access: "my-access", Refresh: "my-refresh", AccountID: "acct-my"}, true); err != nil {
		t.Fatalf("save my profile: %v", err)
	}
	if err := saveState(paths, StateFile{Version: 1, ActiveProfile: "my"}); err != nil {
		t.Fatalf("save state: %v", err)
	}

	svc := NewService()
	results, err := svc.Status([]ToolName{ToolOpenClaw})
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one status result, got %d", len(results))
	}
	if results[0].ActiveProfile != "" {
		t.Fatalf("expected stale active profile hidden, got %q", results[0].ActiveProfile)
	}
}

func TestStatusOpenClawShowsVerifiedActiveProfile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", filepath.Join(tmp, "home"))
	t.Setenv("OPENCLAW_AGENT_DIR", filepath.Join(tmp, "agent"))

	paths, err := resolveToolPaths(ToolOpenClaw)
	if err != nil {
		t.Fatalf("resolve paths: %v", err)
	}

	active := Credential{Provider: "openai-codex", Access: "my-access", Refresh: "my-refresh", AccountID: "acct-my"}
	if err := writeJSONAtomic(paths.ActivePath, map[string]any{
		"version": 1,
		"profiles": map[string]any{
			openClawManagedProfileID: map[string]any{
				"type":      "oauth",
				"provider":  "openai-codex",
				"access":    active.Access,
				"refresh":   active.Refresh,
				"accountId": active.AccountID,
			},
		},
		"order": map[string]any{
			"openai-codex": []string{openClawManagedProfileID},
		},
	}); err != nil {
		t.Fatalf("write openclaw active store: %v", err)
	}
	if err := saveState(paths, StateFile{Version: 1, ActiveProfile: "my", ActiveCredentialHash: credentialFingerprint(active)}); err != nil {
		t.Fatalf("save state: %v", err)
	}

	svc := NewService()
	results, err := svc.Status([]ToolName{ToolOpenClaw})
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one status result, got %d", len(results))
	}
	if results[0].ActiveProfile != "my" {
		t.Fatalf("expected verified active profile my, got %q", results[0].ActiveProfile)
	}
}
