package app

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

type openClawAdapter struct{}

func (a *openClawAdapter) Tool() ToolName { return ToolOpenClaw }

type openClawCredential struct {
	Type     string `json:"type"`
	Provider string `json:"provider"`

	Access    string `json:"access,omitempty"`
	Refresh   string `json:"refresh,omitempty"`
	Expires   int64  `json:"expires,omitempty"`
	AccountID string `json:"accountId,omitempty"`
	ClientID  string `json:"clientId,omitempty"`
	Email     string `json:"email,omitempty"`

	Token string `json:"token,omitempty"`
}

type openClawStore struct {
	Version  int                           `json:"version"`
	Profiles map[string]openClawCredential `json:"profiles"`
	Order    map[string][]string           `json:"order,omitempty"`

	Raw map[string]any `json:"-"`
}

func (a *openClawAdapter) Inspect(paths ToolPaths) (InspectToolResult, error) {
	out := InspectToolResult{Tool: a.Tool(), Paths: paths}
	cred, ok, err := a.ReadActiveCredential(paths)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return out, err
	}
	out.HasActive = ok
	out.Capturable = ok && cred.Access != "" && cred.Refresh != ""
	out.AccountID = cred.AccountID
	out.Email = cred.Email
	out.Expires = cred.Expires
	return out, nil
}

func (a *openClawAdapter) ReadActiveCredential(paths ToolPaths) (Credential, bool, error) {
	store, err := readOpenClawStore(paths.ActivePath)
	if err != nil {
		return Credential{}, false, err
	}

	order := activeOpenClawOrder(store)
	now := time.Now().UnixMilli()
	for _, id := range order {
		entry, ok := store.Profiles[id]
		if !ok {
			continue
		}
		if strings.ToLower(strings.TrimSpace(entry.Provider)) != "openai-codex" {
			continue
		}
		if entry.Type == "oauth" {
			if entry.Access == "" || entry.Refresh == "" {
				continue
			}
			return Credential{
				Provider:  "openai-codex",
				Access:    entry.Access,
				Refresh:   entry.Refresh,
				Expires:   entry.Expires,
				AccountID: entry.AccountID,
				Email:     entry.Email,
			}, true, nil
		}
		if entry.Type == "token" {
			if entry.Token == "" {
				continue
			}
			if entry.Expires > 0 && now >= entry.Expires {
				continue
			}
			return Credential{
				Provider: "openai-codex",
				Access:   entry.Token,
				Expires:  entry.Expires,
				Email:    entry.Email,
			}, true, nil
		}
	}

	return Credential{}, false, nil
}

func (a *openClawAdapter) WriteActiveCredential(paths ToolPaths, cred Credential) error {
	if cred.Access == "" || cred.Refresh == "" {
		return fmt.Errorf("openclaw credential requires access and refresh token")
	}
	return a.WriteWithProfile(paths, "__active__", cred)
}

func (a *openClawAdapter) ClearActiveCredential(paths ToolPaths) error {
	store, err := readOpenClawStore(paths.ActivePath)
	if err != nil {
		if os.IsNotExist(err) {
			store = openClawStore{Version: 1, Profiles: map[string]openClawCredential{}, Order: map[string][]string{}, Raw: map[string]any{}}
		} else {
			return err
		}
	}
	if store.Profiles == nil {
		store.Profiles = map[string]openClawCredential{}
	}
	if store.Order == nil {
		store.Order = map[string][]string{}
	}

	for id, entry := range store.Profiles {
		provider := strings.ToLower(strings.TrimSpace(entry.Provider))
		if provider != "openai-codex" {
			continue
		}
		if entry.Type == "oauth" || entry.Type == "token" {
			delete(store.Profiles, id)
		}
	}
	delete(store.Order, "openai-codex")

	if store.Version == 0 {
		store.Version = 1
	}
	return writeOpenClawStore(paths.ActivePath, store)
}

func (a *openClawAdapter) WriteWithProfile(paths ToolPaths, profileName string, cred Credential) error {
	store, err := readOpenClawStore(paths.ActivePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if store.Profiles == nil {
		store.Profiles = map[string]openClawCredential{}
	}
	if store.Order == nil {
		store.Order = map[string][]string{}
	}

	profileID := "openai-codex:rotater:" + profileName
	store.Profiles[profileID] = openClawCredential{
		Type:      "oauth",
		Provider:  "openai-codex",
		Access:    cred.Access,
		Refresh:   cred.Refresh,
		Expires:   cred.Expires,
		AccountID: cred.AccountID,
		ClientID:  "app_EMoamEEZ73f0CkXaXp7hrann",
		Email:     cred.Email,
	}

	existing := store.Order["openai-codex"]
	merged := []string{profileID}
	for _, id := range existing {
		if id == profileID {
			continue
		}
		merged = append(merged, id)
	}
	store.Order["openai-codex"] = dedupeStrings(merged)

	if store.Version == 0 {
		store.Version = 1
	}
	return writeOpenClawStore(paths.ActivePath, store)
}

func readOpenClawStore(path string) (openClawStore, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return openClawStore{}, err
	}
	var raw map[string]any
	if err := json.Unmarshal(bytes, &raw); err != nil {
		return openClawStore{}, err
	}
	store := openClawStore{Raw: raw}
	store.Version = int(toInt64(raw["version"]))

	store.Profiles = map[string]openClawCredential{}
	if profiles, ok := raw["profiles"].(map[string]any); ok {
		for id, value := range profiles {
			bytes, _ := json.Marshal(value)
			var c openClawCredential
			if err := json.Unmarshal(bytes, &c); err == nil {
				store.Profiles[id] = c
			}
		}
	}

	store.Order = map[string][]string{}
	if order, ok := raw["order"].(map[string]any); ok {
		for provider, listRaw := range order {
			listAny, ok := listRaw.([]any)
			if !ok {
				continue
			}
			list := make([]string, 0, len(listAny))
			for _, item := range listAny {
				s, ok := item.(string)
				if ok && strings.TrimSpace(s) != "" {
					list = append(list, s)
				}
			}
			store.Order[provider] = list
		}
	}

	return store, nil
}

func writeOpenClawStore(path string, store openClawStore) error {
	raw := store.Raw
	if raw == nil {
		raw = map[string]any{}
	}
	raw["version"] = store.Version
	raw["profiles"] = store.Profiles
	if len(store.Order) > 0 {
		raw["order"] = store.Order
	}
	return writeJSONAtomic(path, raw)
}

func activeOpenClawOrder(store openClawStore) []string {
	if list := store.Order["openai-codex"]; len(list) > 0 {
		return dedupeStrings(list)
	}
	ids := make([]string, 0)
	for id, cred := range store.Profiles {
		if strings.ToLower(strings.TrimSpace(cred.Provider)) == "openai-codex" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func dedupeStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, v := range values {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
