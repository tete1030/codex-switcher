package app

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestUsageDefaultMaterializesPendingCreateProfile(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	codexHome := filepath.Join(home, ".codex")
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", codexHome)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		_, _ = w.Write([]byte(`{"plan_type":"plus","credits":{"balance":0},"rate_limit":{"primary_window":{"limit_window_seconds":18000,"used_percent":1,"reset_at":1773000000},"secondary_window":{"limit_window_seconds":86400,"used_percent":2,"reset_at":1773086400}}}`))
	}))
	defer server.Close()
	t.Setenv("CODEX_SWITCHER_USAGE_URL", server.URL+"/backend-api/wham/usage")

	paths, err := resolveToolPaths(ToolCodex)
	if err != nil {
		t.Fatalf("resolve paths: %v", err)
	}

	if err := writeJSONAtomic(paths.ActivePath, map[string]any{
		"auth_mode": "chatgpt",
		"tokens": map[string]any{
			"access_token":  "access-new",
			"refresh_token": "refresh-new",
			"account_id":    "acct-new",
		},
	}); err != nil {
		t.Fatalf("write auth: %v", err)
	}

	if err := saveProfile(paths, "my", Credential{
		Provider:  "openai-codex",
		Access:    "old-access",
		Refresh:   "old-refresh",
		AccountID: "old-acct",
	}, true); err != nil {
		t.Fatalf("save old profile: %v", err)
	}

	if err := saveState(paths, StateFile{
		Version:              1,
		ActiveProfile:        "",
		PreviousProfile:      "my",
		PendingCreateProfile: "buy1",
		PendingCreateSince:   "2026-01-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("save state: %v", err)
	}

	svc := NewService()
	results, err := svc.Usage(UsageOptions{})
	if err != nil {
		t.Fatalf("usage failed: %v", err)
	}

	found := false
	for _, result := range results {
		if result.Tool == ToolCodex && result.Profile == "buy1" && result.Status == "ok" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected codex/buy1 usage result, got %+v", results)
	}

	buy1, err := loadProfile(paths, "buy1")
	if err != nil {
		t.Fatalf("load buy1 profile: %v", err)
	}
	if buy1.Access != "access-new" || buy1.Refresh != "refresh-new" || buy1.AccountID != "acct-new" {
		t.Fatalf("expected buy1 profile synced from active auth, got %+v", buy1)
	}

	updatedState, err := loadState(paths)
	if err != nil {
		t.Fatalf("load updated state: %v", err)
	}
	if updatedState.PendingCreateProfile != "" {
		t.Fatalf("expected pending create cleared, got %q", updatedState.PendingCreateProfile)
	}
	if updatedState.ActiveProfile != "buy1" {
		t.Fatalf("expected active profile buy1, got %q", updatedState.ActiveProfile)
	}
}
