package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultUsageURL    = "https://chatgpt.com/backend-api/wham/usage"
	refreshURL         = "https://auth.openai.com/oauth/token"
	oauthClientID      = "app_EMoamEEZ73f0CkXaXp7hrann"
	refreshThreshold   = 30 * time.Second
	defaultHTTPTimeout = 12 * time.Second
	unknownProfileName = "-"
)

type UsageOptions struct {
	Profile     string
	AllProfiles bool
	Tools       []ToolName
}

func (s *Service) Usage(opts UsageOptions) ([]UsageResult, error) {
	if opts.Profile != "" {
		if err := validateProfileName(opts.Profile); err != nil {
			return nil, WrapExit(ExitUserError, err)
		}
	}

	tools, err := resolveUsageTools(opts.Tools)
	if err != nil {
		return nil, WrapExit(ExitUserError, err)
	}

	httpClient := &http.Client{Timeout: defaultHTTPTimeout}
	usageURL := firstNonEmpty(strings.TrimSpace(os.Getenv("CODEX_SWITCHER_USAGE_URL")), defaultUsageURL)
	defaultActiveQuery := opts.Profile == "" && len(opts.Tools) == 0 && !opts.AllProfiles

	results := make([]UsageResult, 0)
	for _, tool := range tools {
		paths, pathErr := resolveToolPaths(tool)
		if pathErr != nil {
			results = append(results, UsageResult{
				Tool:     tool,
				Profile:  unknownProfileName,
				Provider: "openai-codex",
				Status:   "error",
				Error:    pathErr.Error(),
			})
			continue
		}
		adapter := adapterFor(tool)
		if adapter == nil {
			results = append(results, UsageResult{
				Tool:     tool,
				Profile:  unknownProfileName,
				Provider: "openai-codex",
				Status:   "error",
				Error:    "unsupported tool adapter",
			})
			continue
		}

		profilesToLoad, selectErr := selectUsageProfilesForTool(opts, paths)
		if selectErr != nil {
			results = append(results, UsageResult{
				Tool:     tool,
				Profile:  unknownProfileName,
				Provider: "openai-codex",
				Status:   "error",
				Error:    selectErr.Error(),
			})
			continue
		}
		if len(profilesToLoad) == 0 {
			results = append(results, UsageResult{
				Tool:     tool,
				Profile:  unknownProfileName,
				Provider: "openai-codex",
				Status:   "error",
				Error:    "no profiles available for this tool",
			})
			continue
		}

		state, _ := loadState(paths)
		hadSuccess := false
		lastSuccessProfile := ""

		for _, name := range profilesToLoad {
			preferredActiveProfile := ""
			if defaultActiveQuery && name == "__active__" {
				preferredActiveProfile = firstNonEmpty(state.ActiveProfile, state.PendingCreateProfile)
			}

			cred, sourceLabel, resolveErr := resolveUsageCredential(paths, adapter, name, preferredActiveProfile)
			if resolveErr != nil {
				results = append(results, UsageResult{
					Tool:     tool,
					Profile:  usageProfileLabel(name),
					Provider: "openai-codex",
					Status:   "error",
					Error:    resolveErr.Error(),
				})
				continue
			}

			result, newCred, refreshed, usageErr := fetchUsageWithRefresh(httpClient, usageURL, cred)
			if usageErr != nil {
				status := "error"
				if refreshed {
					status = "auth_error"
				}
				results = append(results, UsageResult{
					Tool:      tool,
					Profile:   sourceLabel,
					Provider:  "openai-codex",
					AccountID: cred.AccountID,
					Status:    status,
					Error:     usageErr.Error(),
					Refreshed: refreshed,
				})
				continue
			}

			result.Tool = tool
			result.Profile = sourceLabel
			if result.AccountID == "" {
				result.AccountID = cred.AccountID
			}
			result.Refreshed = refreshed
			results = append(results, result)
			hadSuccess = true
			if name != "__active__" {
				lastSuccessProfile = name
			} else if sourceLabel != unknownProfileName {
				lastSuccessProfile = sourceLabel
			}

			if refreshed {
				if name != "__active__" {
					_ = saveProfile(paths, name, newCred, true)
					stateAfterRefresh, _ := loadState(paths)
					if stateAfterRefresh.ActiveProfile == name {
						if tool == ToolOpenClaw {
							oa, ok := adapter.(*openClawAdapter)
							if ok {
								_ = oa.WriteWithProfile(paths, name, newCred)
							}
						} else {
							_ = adapter.WriteActiveCredential(paths, newCred)
						}
					}
				} else {
					if sourceLabel != unknownProfileName {
						_ = saveProfile(paths, sourceLabel, newCred, true)
					}
					_ = adapter.WriteActiveCredential(paths, newCred)
				}
			}

			if name == "__active__" && sourceLabel != unknownProfileName {
				synced := cred
				if refreshed {
					synced = newCred
				}
				targetProfile := sourceLabel
				if targetProfile == unknownProfileName {
					targetProfile = state.PendingCreateProfile
				}
				if targetProfile != "" && targetProfile != unknownProfileName {
					_ = saveProfile(paths, targetProfile, synced, true)
				}
			}
		}

		if hadSuccess && state.PendingCreateProfile != "" {
			pendingProfile := state.PendingCreateProfile
			state.PendingCreateProfile = ""
			state.PendingCreateSince = ""
			if opts.Profile != "" {
				state.ActiveProfile = opts.Profile
			} else if lastSuccessProfile != "" {
				state.ActiveProfile = lastSuccessProfile
			} else if state.ActiveProfile == "" {
				state.ActiveProfile = pendingProfile
			}
			_ = saveState(paths, state)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Tool == results[j].Tool {
			return results[i].Profile < results[j].Profile
		}
		return results[i].Tool < results[j].Tool
	})
	return results, nil
}

func resolveUsageTools(selected []ToolName) ([]ToolName, error) {
	if len(selected) == 0 {
		return append([]ToolName{}, AllTools...), nil
	}
	tools := make([]ToolName, 0, len(selected))
	seen := map[ToolName]struct{}{}
	for _, tool := range selected {
		if tool != ToolCodex && tool != ToolOpenCode && tool != ToolOpenClaw {
			return nil, fmt.Errorf("unsupported tool %s", tool)
		}
		if _, ok := seen[tool]; ok {
			continue
		}
		seen[tool] = struct{}{}
		tools = append(tools, tool)
	}
	return tools, nil
}

func selectUsageProfilesForTool(opts UsageOptions, paths ToolPaths) ([]string, error) {
	if opts.Profile != "" {
		return []string{opts.Profile}, nil
	}

	if len(opts.Tools) > 0 {
		list, err := listProfiles(paths)
		if err != nil {
			return nil, err
		}
		return list, nil
	}

	if opts.AllProfiles {
		list, err := listProfiles(paths)
		if err != nil {
			return nil, err
		}
		return list, nil
	}

	return []string{"__active__"}, nil
}

func usageProfileLabel(name string) string {
	if name == "__active__" {
		return unknownProfileName
	}
	return name
}

func resolveUsageCredential(paths ToolPaths, adapter Adapter, name string, preferredActiveProfile string) (Credential, string, error) {
	if name == "__active__" {
		cred, ok, err := adapter.ReadActiveCredential(paths)
		if err != nil {
			return Credential{}, unknownProfileName, err
		}
		if !ok {
			return Credential{}, unknownProfileName, fmt.Errorf("no active credential available")
		}
		if preferredActiveProfile != "" {
			return cred, preferredActiveProfile, nil
		}
		if matched := guessActiveProfileName(paths, cred); matched != "" {
			return cred, matched, nil
		}
		return cred, unknownProfileName, nil
	}
	cred, err := loadProfile(paths, name)
	if err != nil {
		return Credential{}, name, err
	}
	return cred, name, nil
}

func guessActiveProfileName(paths ToolPaths, active Credential) string {
	profiles, err := listProfiles(paths)
	if err != nil {
		return ""
	}
	for _, name := range profiles {
		profileCred, loadErr := loadProfile(paths, name)
		if loadErr != nil {
			continue
		}
		if credentialsLikelyMatch(active, profileCred) {
			return name
		}
	}
	return ""
}

func credentialsLikelyMatch(a Credential, b Credential) bool {
	if strings.TrimSpace(a.Provider) != "" && strings.TrimSpace(b.Provider) != "" && a.Provider != b.Provider {
		return false
	}
	if a.Refresh != "" && b.Refresh != "" && a.Refresh == b.Refresh {
		return true
	}
	if a.Access != "" && b.Access != "" && a.Access == b.Access {
		return true
	}
	return false
}

func fetchUsageWithRefresh(client *http.Client, usageURL string, cred Credential) (UsageResult, Credential, bool, error) {
	current := cred
	refreshed := false
	now := time.Now()

	if current.NearExpiry(now, refreshThreshold) {
		next, err := refreshCredential(client, current)
		if err == nil {
			current = next
			refreshed = true
		}
	}

	result, statusCode, err := fetchUsage(client, usageURL, current)
	if err == nil {
		return result, current, refreshed, nil
	}

	if statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden {
		next, refreshErr := refreshCredential(client, current)
		if refreshErr != nil {
			return UsageResult{}, cred, refreshed, fmt.Errorf("usage unauthorized and refresh failed: %w", refreshErr)
		}
		current = next
		refreshed = true

		retryResult, _, retryErr := fetchUsage(client, usageURL, current)
		if retryErr != nil {
			return UsageResult{}, current, refreshed, retryErr
		}
		return retryResult, current, refreshed, nil
	}

	return UsageResult{}, current, refreshed, err
}

func fetchUsage(client *http.Client, usageURL string, cred Credential) (UsageResult, int, error) {
	result, status, err := fetchUsageOnce(client, usageURL, cred)
	if err == nil {
		return result, status, nil
	}
	if status == http.StatusNotFound && strings.Contains(usageURL, "/wham/usage") {
		fallbackURL := strings.Replace(usageURL, "/wham/usage", "/api/codex/usage", 1)
		return fetchUsageOnce(client, fallbackURL, cred)
	}
	return UsageResult{}, status, err
}

func fetchUsageOnce(client *http.Client, usageURL string, cred Credential) (UsageResult, int, error) {
	req, err := http.NewRequest(http.MethodGet, usageURL, nil)
	if err != nil {
		return UsageResult{}, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+cred.Access)
	req.Header.Set("User-Agent", "codex-switcher")
	req.Header.Set("Accept", "application/json")
	if cred.AccountID != "" {
		req.Header.Set("ChatGPT-Account-Id", cred.AccountID)
	}

	res, err := client.Do(req)
	if err != nil {
		return UsageResult{}, 0, err
	}
	defer res.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(res.Body, 2*1024*1024))
	if res.StatusCode < 200 || res.StatusCode > 299 {
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = http.StatusText(res.StatusCode)
		}
		return UsageResult{}, res.StatusCode, fmt.Errorf("usage request failed (%d): %s", res.StatusCode, msg)
	}

	parsed, err := normalizeUsage(body)
	if err != nil {
		return UsageResult{}, res.StatusCode, err
	}
	parsed.Provider = "openai-codex"
	parsed.Status = "ok"
	return parsed, res.StatusCode, nil
}

func normalizeUsage(body []byte) (UsageResult, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return UsageResult{}, fmt.Errorf("failed to parse usage response: %w", err)
	}

	windows := []UsageWindow{}
	if rateLimit, ok := payload["rate_limit"].(map[string]any); ok {
		if primary, ok := rateLimit["primary_window"].(map[string]any); ok {
			windowHours := toInt64(primary["limit_window_seconds"]) / 3600
			if windowHours <= 0 {
				windowHours = 3
			}
			window := UsageWindow{
				Label:       fmt.Sprintf("%dh", windowHours),
				UsedPercent: clampPercent(toFloat(primary["used_percent"])),
				ResetAt:     toInt64(primary["reset_at"]) * 1000,
			}
			enrichUsageWindow(&window)
			windows = append(windows, window)
		}
		if secondary, ok := rateLimit["secondary_window"].(map[string]any); ok {
			windowHours := toInt64(secondary["limit_window_seconds"]) / 3600
			if windowHours <= 0 {
				windowHours = 24
			}
			label := fmt.Sprintf("%dh", windowHours)
			if windowHours >= 24 {
				label = "Day"
			}
			window := UsageWindow{
				Label:       label,
				UsedPercent: clampPercent(toFloat(secondary["used_percent"])),
				ResetAt:     toInt64(secondary["reset_at"]) * 1000,
			}
			enrichUsageWindow(&window)
			windows = append(windows, window)
		}
	}

	plan, _ := payload["plan_type"].(string)
	var credits *float64
	if credNode, ok := payload["credits"].(map[string]any); ok {
		balanceValue, exists := credNode["balance"]
		if exists {
			v := toFloat(balanceValue)
			credits = &v
		}
	}

	return UsageResult{
		Provider:       "openai-codex",
		Plan:           plan,
		CreditsBalance: credits,
		Windows:        windows,
		Status:         "ok",
	}, nil
}

func refreshCredential(client *http.Client, cred Credential) (Credential, error) {
	if cred.Refresh == "" {
		return Credential{}, fmt.Errorf("missing refresh token")
	}

	values := url.Values{}
	values.Set("grant_type", "refresh_token")
	values.Set("refresh_token", cred.Refresh)
	values.Set("client_id", oauthClientID)
	values.Set("scope", "openid profile email")

	req, err := http.NewRequest(http.MethodPost, refreshURL, bytes.NewBufferString(values.Encode()))
	if err != nil {
		return Credential{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "codex-switcher")

	res, err := client.Do(req)
	if err != nil {
		return Credential{}, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 2*1024*1024))

	if res.StatusCode < 200 || res.StatusCode > 299 {
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = http.StatusText(res.StatusCode)
		}
		return Credential{}, fmt.Errorf("refresh failed (%d): %s", res.StatusCode, msg)
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return Credential{}, err
	}
	access, _ := payload["access_token"].(string)
	if access == "" {
		return Credential{}, fmt.Errorf("refresh response missing access_token")
	}
	refresh, _ := payload["refresh_token"].(string)
	if refresh == "" {
		refresh = cred.Refresh
	}
	idToken, _ := payload["id_token"].(string)
	expiresIn := toInt64(payload["expires_in"])
	if expiresIn <= 0 {
		expiresIn = 3600
	}
	expires := time.Now().Add(time.Duration(expiresIn) * time.Second).UnixMilli()

	accountID := cred.AccountID
	email := cred.Email
	claims := parseJWTClaims(idToken)
	if claims == nil {
		claims = parseJWTClaims(access)
	}
	if claims != nil {
		if account := extractAccountID(claims); account != "" {
			accountID = account
		}
		if parsedEmail := extractEmail(claims); parsedEmail != "" {
			email = parsedEmail
		}
	}

	return Credential{
		Provider:  "openai-codex",
		Access:    access,
		Refresh:   refresh,
		Expires:   expires,
		AccountID: accountID,
		IDToken:   idToken,
		Email:     email,
		UpdatedAt: time.Now().UnixMilli(),
	}, nil
}

func clampPercent(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

func enrichUsageWindow(window *UsageWindow) {
	if window == nil || window.ResetAt <= 0 {
		return
	}
	reset := time.UnixMilli(window.ResetAt).UTC()
	window.ResetAtISO = reset.Format(time.RFC3339)
	remaining := time.Until(reset)
	if remaining <= 0 {
		window.RemainingDays = 0
		return
	}
	window.RemainingDays = float64(remaining) / float64(24*time.Hour)
}

func toFloat(value any) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case json.Number:
		f, _ := v.Float64()
		return f
	case string:
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return 0
		}
		return f
	default:
		return 0
	}
}
