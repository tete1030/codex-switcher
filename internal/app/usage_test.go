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

func TestUsageWithExplicitToolsDoesNotClearPendingCreateFromOtherProfileQuery(t *testing.T) {
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
			"access_token":  "active-buy1-access",
			"refresh_token": "active-buy1-refresh",
			"account_id":    "active-buy1-acct",
		},
	}); err != nil {
		t.Fatalf("write auth: %v", err)
	}

	if err := saveProfile(paths, "my", Credential{
		Provider:  "openai-codex",
		Access:    "my-access",
		Refresh:   "my-refresh",
		AccountID: "my-acct",
	}, true); err != nil {
		t.Fatalf("save my profile: %v", err)
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
	results, err := svc.Usage(UsageOptions{Tools: []ToolName{ToolCodex}})
	if err != nil {
		t.Fatalf("usage failed: %v", err)
	}

	foundMy := false
	for _, result := range results {
		if result.Tool == ToolCodex && result.Profile == "my" && result.Status == "ok" {
			foundMy = true
		}
		if result.Tool == ToolCodex && result.Profile == "buy1" {
			t.Fatalf("did not expect buy1 profile to be materialized for explicit tools query")
		}
	}
	if !foundMy {
		t.Fatalf("expected codex/my usage result, got %+v", results)
	}

	if _, err := loadProfile(paths, "buy1"); err == nil {
		t.Fatalf("did not expect buy1 profile file to be created")
	}

	updatedState, err := loadState(paths)
	if err != nil {
		t.Fatalf("load updated state: %v", err)
	}
	if updatedState.PendingCreateProfile != "buy1" {
		t.Fatalf("expected pending create preserved as buy1, got %q", updatedState.PendingCreateProfile)
	}
	if updatedState.ActiveProfile != "" {
		t.Fatalf("expected active profile unchanged empty, got %q", updatedState.ActiveProfile)
	}
}

func TestUsageDefaultDoesNotTrustStaleActiveProfileLabel(t *testing.T) {
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
			"access_token":  "buy1-access",
			"refresh_token": "buy1-refresh",
			"account_id":    "acct-buy1",
		},
	}); err != nil {
		t.Fatalf("write auth: %v", err)
	}

	if err := saveProfile(paths, "my", Credential{
		Provider:  "openai-codex",
		Access:    "my-access",
		Refresh:   "my-refresh",
		AccountID: "acct-my",
	}, true); err != nil {
		t.Fatalf("save my profile: %v", err)
	}
	if err := saveProfile(paths, "buy1", Credential{
		Provider:  "openai-codex",
		Access:    "buy1-access",
		Refresh:   "buy1-refresh",
		AccountID: "acct-buy1",
	}, true); err != nil {
		t.Fatalf("save buy1 profile: %v", err)
	}

	if err := saveState(paths, StateFile{
		Version:         1,
		ActiveProfile:   "my", // stale label, does not match active tokens
		PreviousProfile: "buy1",
	}); err != nil {
		t.Fatalf("save state: %v", err)
	}

	svc := NewService()
	results, err := svc.Usage(UsageOptions{})
	if err != nil {
		t.Fatalf("usage failed: %v", err)
	}

	foundBuy1 := false
	foundMy := false
	for _, result := range results {
		if result.Tool != ToolCodex {
			continue
		}
		if result.Profile == "buy1" && result.Status == "ok" {
			foundBuy1 = true
		}
		if result.Profile == "my" && result.Status == "ok" {
			foundMy = true
		}
	}
	if !foundBuy1 {
		t.Fatalf("expected codex/buy1 result, got %+v", results)
	}
	if foundMy {
		t.Fatalf("did not expect codex/my result when active tokens match buy1")
	}
}
