package app

import (
	"path/filepath"
	"testing"
)

func TestOpenClawClearActiveKeepsProfilesButClearsActiveOrder(t *testing.T) {
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
			"openai-codex": []string{openClawManagedProfileID, openClawLegacyPendingLoginSentinelID, "openai-codex:manual"},
		},
		openClawLegacyPendingKnownIDsKey: []string{"openai-codex:manual"},
	}
	if err := writeJSONAtomic(paths.ActivePath, store); err != nil {
		t.Fatalf("write store: %v", err)
	}

	adapter := &openClawAdapter{}
	if err := adapter.ClearActiveCredential(paths); err != nil {
		t.Fatalf("clear active: %v", err)
	}

	updated, err := readOpenClawStore(paths.ActivePath)
	if err != nil {
		t.Fatalf("read store: %v", err)
	}
	if _, ok := updated.Profiles["openai-codex:manual"]; !ok {
		t.Fatalf("expected manual profile preserved")
	}
	if _, ok := updated.Profiles[openClawManagedProfileID]; ok {
		t.Fatalf("expected managed profile removed")
	}
	order := updated.Order["openai-codex"]
	if len(order) != 0 {
		t.Fatalf("expected empty active order after clear, got %+v", order)
	}
	if _, ok := updated.Raw[openClawLegacyPendingKnownIDsKey]; ok {
		t.Fatalf("expected legacy pending-known marker removed")
	}

	cred, ok, err := adapter.ReadActiveCredential(paths)
	if err != nil {
		t.Fatalf("read active credential: %v", err)
	}
	if ok {
		t.Fatalf("expected no active credential after clear, got %+v", cred)
	}
}

func TestOpenClawWriteWithProfileUsesSingleManagedProfile(t *testing.T) {
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
				"access":    "old-access",
				"refresh":   "old-refresh",
				"accountId": "acct-old",
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
		t.Fatalf("write store: %v", err)
	}

	adapter := &openClawAdapter{}
	if err := adapter.WriteWithProfile(paths, "buy2", Credential{Provider: "openai-codex", Access: "new-access", Refresh: "new-refresh", AccountID: "acct-new"}); err != nil {
		t.Fatalf("write active: %v", err)
	}

	updated, err := readOpenClawStore(paths.ActivePath)
	if err != nil {
		t.Fatalf("read store: %v", err)
	}
	managed, ok := updated.Profiles[openClawManagedProfileID]
	if !ok {
		t.Fatalf("expected managed profile present")
	}
	if managed.Access != "new-access" || managed.Refresh != "new-refresh" || managed.AccountID != "acct-new" {
		t.Fatalf("expected managed credentials updated, got %+v", managed)
	}
	if _, ok := updated.Profiles["openai-codex:rotater:buy1"]; ok {
		t.Fatalf("expected legacy rotater profile removed")
	}
	if _, ok := updated.Profiles["openai-codex:manual"]; !ok {
		t.Fatalf("expected manual profile preserved")
	}
	order := updated.Order["openai-codex"]
	if len(order) != 1 || order[0] != openClawManagedProfileID {
		t.Fatalf("expected single managed active order, got %+v", order)
	}
	if _, ok := updated.Raw[openClawLegacyPendingKnownIDsKey]; ok {
		t.Fatalf("expected legacy pending-known marker removed")
	}
}

func TestOpenClawReadActiveCredentialFindsNewLoginAfterClear(t *testing.T) {
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
				"access":    "old-access",
				"refresh":   "old-refresh",
				"accountId": "acct-old",
			},
		},
		"order": map[string]any{
			"openai-codex": []string{openClawManagedProfileID},
		},
	}
	if err := writeJSONAtomic(paths.ActivePath, store); err != nil {
		t.Fatalf("write store: %v", err)
	}

	adapter := &openClawAdapter{}
	if err := adapter.ClearActiveCredential(paths); err != nil {
		t.Fatalf("clear active: %v", err)
	}

	updated, err := readOpenClawStore(paths.ActivePath)
	if err != nil {
		t.Fatalf("read store: %v", err)
	}
	updated.Profiles["openai-codex:oauthed:new"] = openClawCredential{
		Type:      "oauth",
		Provider:  "openai-codex",
		Access:    "new-access",
		Refresh:   "new-refresh",
		AccountID: "acct-new",
	}
	updated.Order["openai-codex"] = []string{"openai-codex:oauthed:new"}
	if err := writeOpenClawStore(paths.ActivePath, updated); err != nil {
		t.Fatalf("write updated store: %v", err)
	}

	cred, ok, err := adapter.ReadActiveCredential(paths)
	if err != nil {
		t.Fatalf("read active credential: %v", err)
	}
	if !ok {
		t.Fatalf("expected active credential")
	}
	if cred.AccountID != "acct-new" {
		t.Fatalf("expected account acct-new, got %q", cred.AccountID)
	}
}
