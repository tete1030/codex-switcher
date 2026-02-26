package app

import (
	"path/filepath"
	"testing"
)

func TestOpenClawClearActivePreservesProfilesAndUsesSentinelOrder(t *testing.T) {
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
			"openai-codex:manual": map[string]any{
				"type":      "oauth",
				"provider":  "openai-codex",
				"access":    "manual-access",
				"refresh":   "manual-refresh",
				"accountId": "acct-manual",
			},
			"openai-codex:rotater:my": map[string]any{
				"type":      "oauth",
				"provider":  "openai-codex",
				"access":    "rotater-access",
				"refresh":   "rotater-refresh",
				"accountId": "acct-rotater",
			},
		},
		"order": map[string]any{
			"openai-codex": []string{"openai-codex:manual", "openai-codex:rotater:my"},
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
	if _, ok := updated.Profiles["openai-codex:manual"]; !ok {
		t.Fatalf("expected manual profile preserved")
	}
	if _, ok := updated.Profiles["openai-codex:rotater:my"]; !ok {
		t.Fatalf("expected rotater profile preserved")
	}
	order := updated.Order["openai-codex"]
	if len(order) != 1 || order[0] != openClawPendingLoginSentinelID {
		t.Fatalf("expected sentinel order, got %+v", order)
	}

	cred, ok, err := adapter.ReadActiveCredential(paths)
	if err != nil {
		t.Fatalf("read active credential: %v", err)
	}
	if ok {
		t.Fatalf("expected no active credential after clear, got %+v", cred)
	}
}
