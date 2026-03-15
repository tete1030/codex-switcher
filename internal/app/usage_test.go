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

func TestUsageWithExplicitToolsMaterializesPendingCreateProfileAndPreservesSavedProfiles(t *testing.T) {
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

	if err := saveProfile(paths, "my", Credential{Provider: "openai-codex", Access: "my-access", Refresh: "my-refresh", AccountID: "my-acct"}, true); err != nil {
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

	foundBuy1 := false
	foundMy := false
	for _, result := range results {
		if result.Tool == ToolCodex && result.Profile == "buy1" && result.Status == "ok" {
			foundBuy1 = true
		}
		if result.Tool == ToolCodex && result.Profile == "my" && result.Status == "ok" {
			foundMy = true
		}
	}
	if !foundBuy1 {
		t.Fatalf("expected codex/buy1 usage result, got %+v", results)
	}
	if !foundMy {
		t.Fatalf("expected codex/my usage result to remain visible, got %+v", results)
	}

	buy1, err := loadProfile(paths, "buy1")
	if err != nil {
		t.Fatalf("expected buy1 profile materialized, got err=%v", err)
	}
	if buy1.Access != "active-buy1-access" || buy1.Refresh != "active-buy1-refresh" || buy1.AccountID != "active-buy1-acct" {
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

func TestUsageWithExplicitToolsMaterializesPendingCreateProfileForOpenCodeWithoutHidingSavedProfiles(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	xdgData := filepath.Join(home, ".local", "share")
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", xdgData)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		_, _ = w.Write([]byte(`{"plan_type":"plus","credits":{"balance":0},"rate_limit":{"primary_window":{"limit_window_seconds":18000,"used_percent":1,"reset_at":1773000000},"secondary_window":{"limit_window_seconds":86400,"used_percent":2,"reset_at":1773086400}}}`))
	}))
	defer server.Close()
	t.Setenv("CODEX_SWITCHER_USAGE_URL", server.URL+"/backend-api/wham/usage")

	paths, err := resolveToolPaths(ToolOpenCode)
	if err != nil {
		t.Fatalf("resolve paths: %v", err)
	}

	if err := writeJSONAtomic(paths.ActivePath, map[string]any{
		"openai": map[string]any{
			"type":      "oauth",
			"access":    "active-buy1-access",
			"refresh":   "active-buy1-refresh",
			"accountId": "active-buy1-acct",
		},
	}); err != nil {
		t.Fatalf("write auth: %v", err)
	}

	if err := saveProfile(paths, "my", Credential{Provider: "openai-codex", Access: "my-access", Refresh: "my-refresh", AccountID: "my-acct"}, true); err != nil {
		t.Fatalf("save my profile: %v", err)
	}
	if err := saveProfile(paths, "old2", Credential{Provider: "openai-codex", Access: "old2-access", Refresh: "old2-refresh", AccountID: "old2-acct"}, true); err != nil {
		t.Fatalf("save old2 profile: %v", err)
	}

	if err := saveState(paths, StateFile{Version: 1, PreviousProfile: "my", PendingCreateProfile: "buy1", PendingCreateSince: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatalf("save state: %v", err)
	}

	svc := NewService()
	results, err := svc.Usage(UsageOptions{Tools: []ToolName{ToolOpenCode}})
	if err != nil {
		t.Fatalf("usage failed: %v", err)
	}

	found := map[string]bool{}
	for _, result := range results {
		if result.Tool == ToolOpenCode && result.Status == "ok" {
			found[result.Profile] = true
		}
	}
	for _, name := range []string{"buy1", "my", "old2"} {
		if !found[name] {
			t.Fatalf("expected opencode/%s usage result, got %+v", name, results)
		}
	}

	buy1, err := loadProfile(paths, "buy1")
	if err != nil {
		t.Fatalf("expected buy1 profile materialized, got err=%v", err)
	}
	if buy1.Access != "active-buy1-access" || buy1.Refresh != "active-buy1-refresh" || buy1.AccountID != "active-buy1-acct" {
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

func TestUsageWithExplicitToolsActiveOnlyShowsOnlyActiveProfile(t *testing.T) {
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

	activeCred := Credential{Provider: "openai-codex", Access: "buy1-access", Refresh: "buy1-refresh", AccountID: "buy1-acct"}
	if err := writeJSONAtomic(paths.ActivePath, map[string]any{
		"auth_mode": "chatgpt",
		"tokens": map[string]any{
			"access_token":  activeCred.Access,
			"refresh_token": activeCred.Refresh,
			"account_id":    activeCred.AccountID,
		},
	}); err != nil {
		t.Fatalf("write auth: %v", err)
	}

	if err := saveProfile(paths, "buy1", activeCred, true); err != nil {
		t.Fatalf("save buy1 profile: %v", err)
	}
	if err := saveProfile(paths, "my", Credential{Provider: "openai-codex", Access: "my-access", Refresh: "my-refresh", AccountID: "my-acct"}, true); err != nil {
		t.Fatalf("save my profile: %v", err)
	}
	if err := saveState(paths, StateFile{Version: 1, ActiveProfile: "buy1", ActiveCredentialHash: credentialFingerprint(activeCred)}); err != nil {
		t.Fatalf("save state: %v", err)
	}

	svc := NewService()
	results, err := svc.Usage(UsageOptions{Tools: []ToolName{ToolCodex}, ActiveOnly: true})
	if err != nil {
		t.Fatalf("usage failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected exactly one active-only result, got %+v", results)
	}
	if results[0].Tool != ToolCodex || results[0].Profile != "buy1" || results[0].Status != "ok" {
		t.Fatalf("expected only codex/buy1 active usage, got %+v", results)
	}
}

func TestUsageAllProfilesDoesNotClearPendingCreateFromOtherProfileQuery(t *testing.T) {
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

	if err := saveProfile(paths, "my", Credential{Provider: "openai-codex", Access: "my-access", Refresh: "my-refresh", AccountID: "my-acct"}, true); err != nil {
		t.Fatalf("save my profile: %v", err)
	}

	if err := saveState(paths, StateFile{Version: 1, PreviousProfile: "my", PendingCreateProfile: "buy1", PendingCreateSince: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatalf("save state: %v", err)
	}

	svc := NewService()
	results, err := svc.Usage(UsageOptions{Tools: []ToolName{ToolCodex}, AllProfiles: true})
	if err != nil {
		t.Fatalf("usage failed: %v", err)
	}

	foundMy := false
	for _, result := range results {
		if result.Tool == ToolCodex && result.Profile == "my" && result.Status == "ok" {
			foundMy = true
		}
		if result.Tool == ToolCodex && result.Profile == "buy1" {
			t.Fatalf("did not expect buy1 profile to be materialized for all-profiles query")
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

func TestUsageDefaultOpenClawDoesNotTrustStaleStateLabel(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", filepath.Join(tmp, "home"))
	t.Setenv("OPENCLAW_AGENT_DIR", filepath.Join(tmp, "agent"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		_, _ = w.Write([]byte(`{"plan_type":"plus","credits":{"balance":0},"rate_limit":{"primary_window":{"limit_window_seconds":18000,"used_percent":1,"reset_at":1773000000},"secondary_window":{"limit_window_seconds":86400,"used_percent":2,"reset_at":1773086400}}}`))
	}))
	defer server.Close()
	t.Setenv("CODEX_SWITCHER_USAGE_URL", server.URL+"/backend-api/wham/usage")

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
	if err := saveProfile(paths, "buy1", Credential{Provider: "openai-codex", Access: "buy1-access", Refresh: "buy1-refresh", AccountID: "acct-buy1"}, true); err != nil {
		t.Fatalf("save buy1 profile: %v", err)
	}
	if err := saveState(paths, StateFile{Version: 1, ActiveProfile: "my", PreviousProfile: "buy1"}); err != nil {
		t.Fatalf("save state: %v", err)
	}

	svc := NewService()
	results, err := svc.Usage(UsageOptions{})
	if err != nil {
		t.Fatalf("usage failed: %v", err)
	}

	foundOpenClaw := false
	for _, result := range results {
		if result.Tool != ToolOpenClaw {
			continue
		}
		if result.Status == "ok" {
			foundOpenClaw = true
			if result.Profile != "buy1" {
				t.Fatalf("expected openclaw profile label buy1, got %q (result=%+v)", result.Profile, result)
			}
		}
	}
	if !foundOpenClaw {
		t.Fatalf("expected openclaw usage result, got %+v", results)
	}
}

func TestUsageDefaultOpenClawUsesVerifiedStateTracking(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", filepath.Join(tmp, "home"))
	t.Setenv("OPENCLAW_AGENT_DIR", filepath.Join(tmp, "agent"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		_, _ = w.Write([]byte(`{"plan_type":"plus","credits":{"balance":0},"rate_limit":{"primary_window":{"limit_window_seconds":18000,"used_percent":1,"reset_at":1773000000},"secondary_window":{"limit_window_seconds":86400,"used_percent":2,"reset_at":1773086400}}}`))
	}))
	defer server.Close()
	t.Setenv("CODEX_SWITCHER_USAGE_URL", server.URL+"/backend-api/wham/usage")

	paths, err := resolveToolPaths(ToolOpenClaw)
	if err != nil {
		t.Fatalf("resolve paths: %v", err)
	}

	active := Credential{Provider: "openai-codex", Access: "managed-access", Refresh: "managed-refresh", AccountID: "acct-managed"}
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

	if err := saveProfile(paths, "my", Credential{Provider: "openai-codex", Access: "stale-access", Refresh: "stale-refresh", AccountID: "acct-my"}, true); err != nil {
		t.Fatalf("save my profile: %v", err)
	}
	if err := saveState(paths, StateFile{Version: 1, ActiveProfile: "my", ActiveCredentialHash: credentialFingerprint(active)}); err != nil {
		t.Fatalf("save state: %v", err)
	}

	svc := NewService()
	results, err := svc.Usage(UsageOptions{})
	if err != nil {
		t.Fatalf("usage failed: %v", err)
	}

	for _, result := range results {
		if result.Tool == ToolOpenClaw && result.Status == "ok" {
			if result.Profile != "my" {
				t.Fatalf("expected verified tracked label my, got %q (result=%+v)", result.Profile, result)
			}
			return
		}
	}
	t.Fatalf("expected openclaw usage result, got %+v", results)
}

func TestUsageDefaultOpenClawAmbiguousMatchReturnsUnknown(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", filepath.Join(tmp, "home"))
	t.Setenv("OPENCLAW_AGENT_DIR", filepath.Join(tmp, "agent"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		_, _ = w.Write([]byte(`{"plan_type":"plus","credits":{"balance":0},"rate_limit":{"primary_window":{"limit_window_seconds":18000,"used_percent":1,"reset_at":1773000000},"secondary_window":{"limit_window_seconds":86400,"used_percent":2,"reset_at":1773086400}}}`))
	}))
	defer server.Close()
	t.Setenv("CODEX_SWITCHER_USAGE_URL", server.URL+"/backend-api/wham/usage")

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
				"access":    "shared-access",
				"refresh":   "shared-refresh",
				"accountId": "acct-shared",
			},
		},
		"order": map[string]any{
			"openai-codex": []string{openClawManagedProfileID},
		},
	}); err != nil {
		t.Fatalf("write openclaw active store: %v", err)
	}

	shared := Credential{Provider: "openai-codex", Access: "shared-access", Refresh: "shared-refresh", AccountID: "acct-shared"}
	if err := saveProfile(paths, "buy1", shared, true); err != nil {
		t.Fatalf("save buy1 profile: %v", err)
	}
	if err := saveProfile(paths, "my", shared, true); err != nil {
		t.Fatalf("save my profile: %v", err)
	}
	if err := saveState(paths, StateFile{Version: 1, ActiveProfile: "my"}); err != nil {
		t.Fatalf("save state: %v", err)
	}

	svc := NewService()
	results, err := svc.Usage(UsageOptions{})
	if err != nil {
		t.Fatalf("usage failed: %v", err)
	}

	for _, result := range results {
		if result.Tool == ToolOpenClaw && result.Status == "ok" {
			if result.Profile != unknownProfileName {
				t.Fatalf("expected ambiguous openclaw active label to stay unknown, got %q (result=%+v)", result.Profile, result)
			}
			return
		}
	}
	t.Fatalf("expected openclaw usage result, got %+v", results)
}
