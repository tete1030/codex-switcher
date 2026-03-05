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

const openClawManagedProfileID = "openai-codex:default"
const openClawLegacyPendingLoginSentinelID = "openai-codex:rotater:__pending_login__"
const openClawLegacyPendingKnownIDsKey = "codex_switcher_pending_known_profile_ids"

type openClawMigrationStats struct {
	HasStore                  bool
	Changed                   bool
	ManagedProfileSet         bool
	RemovedLegacyRotater      int
	RemovedPendingSentinel    bool
	RemovedPendingKnownMarker bool
}

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

	if entry, ok := store.Profiles[openClawManagedProfileID]; ok {
		if cred, ok := activeCredentialFromOpenClawEntry(entry); ok {
			return cred, true, nil
		}
	}

	for _, id := range activeOpenClawOrder(store) {
		entry, ok := store.Profiles[id]
		if !ok {
			continue
		}
		if cred, ok := activeCredentialFromOpenClawEntry(entry); ok {
			return cred, true, nil
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
	if store.Raw == nil {
		store.Raw = map[string]any{}
	}

	cleanupLegacyOpenClawPendingMarkers(&store)
	delete(store.Profiles, openClawManagedProfileID)
	store.Order["openai-codex"] = []string{}
	delete(store.Raw, openClawLegacyPendingKnownIDsKey)

	if store.Version == 0 {
		store.Version = 1
	}
	return writeOpenClawStore(paths.ActivePath, store)
}

func (a *openClawAdapter) WriteWithProfile(paths ToolPaths, profileName string, cred Credential) error {
	_ = profileName
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
	if store.Raw == nil {
		store.Raw = map[string]any{}
	}

	cleanupLegacyOpenClawPendingMarkers(&store)
	removeLegacySwitcherOpenClawProfiles(&store)
	store.Profiles[openClawManagedProfileID] = openClawCredential{
		Type:      "oauth",
		Provider:  "openai-codex",
		Access:    cred.Access,
		Refresh:   cred.Refresh,
		Expires:   cred.Expires,
		AccountID: cred.AccountID,
		ClientID:  "app_EMoamEEZ73f0CkXaXp7hrann",
		Email:     cred.Email,
	}
	store.Order["openai-codex"] = []string{openClawManagedProfileID}
	delete(store.Raw, openClawLegacyPendingKnownIDsKey)

	if store.Version == 0 {
		store.Version = 1
	}
	return writeOpenClawStore(paths.ActivePath, store)
}

func migrateOpenClawStore(paths ToolPaths) (openClawMigrationStats, error) {
	stats := openClawMigrationStats{}

	store, err := readOpenClawStore(paths.ActivePath)
	if err != nil {
		if os.IsNotExist(err) {
			return stats, nil
		}
		return stats, err
	}
	stats.HasStore = true

	if store.Profiles == nil {
		store.Profiles = map[string]openClawCredential{}
	}
	if store.Order == nil {
		store.Order = map[string][]string{}
	}
	if store.Raw == nil {
		store.Raw = map[string]any{}
	}

	beforeLegacyCount := countLegacySwitcherOpenClawProfiles(store)
	beforeSentinel := orderContainsID(store.Order["openai-codex"], openClawLegacyPendingLoginSentinelID)
	_, beforePendingKnown := store.Raw[openClawLegacyPendingKnownIDsKey]
	beforeManaged, hadManaged := store.Profiles[openClawManagedProfileID]
	beforeOrder := append([]string{}, store.Order["openai-codex"]...)
	beforeVersion := store.Version

	preferred, hasPreferred := selectPreferredOpenClawCredentialEntry(store)

	cleanupLegacyOpenClawPendingMarkers(&store)
	removeLegacySwitcherOpenClawProfiles(&store)

	if hasPreferred {
		store.Profiles[openClawManagedProfileID] = preferred
		store.Order["openai-codex"] = []string{openClawManagedProfileID}
		stats.ManagedProfileSet = true
	} else {
		delete(store.Profiles, openClawManagedProfileID)
		store.Order["openai-codex"] = []string{}
	}

	if store.Version == 0 {
		store.Version = 1
	}
	stats.RemovedLegacyRotater = beforeLegacyCount - countLegacySwitcherOpenClawProfiles(store)
	stats.RemovedPendingSentinel = beforeSentinel && !orderContainsID(store.Order["openai-codex"], openClawLegacyPendingLoginSentinelID)
	_, afterPendingKnown := store.Raw[openClawLegacyPendingKnownIDsKey]
	stats.RemovedPendingKnownMarker = beforePendingKnown && !afterPendingKnown

	if beforeVersion == 0 && store.Version != 0 {
		stats.Changed = true
	}
	if stats.RemovedLegacyRotater > 0 || stats.RemovedPendingSentinel || stats.RemovedPendingKnownMarker {
		stats.Changed = true
	}
	if hasPreferred {
		if !hadManaged || beforeManaged != preferred {
			stats.Changed = true
		}
		if !stringSliceEqual(beforeOrder, []string{openClawManagedProfileID}) {
			stats.Changed = true
		}
	} else {
		if hadManaged {
			stats.Changed = true
		}
		if len(beforeOrder) > 0 {
			stats.Changed = true
		}
	}

	if !stats.Changed {
		return stats, nil
	}
	if err := writeOpenClawStore(paths.ActivePath, store); err != nil {
		return stats, err
	}
	return stats, nil
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
	} else {
		delete(raw, "order")
	}
	return writeJSONAtomic(path, raw)
}

func activeOpenClawOrder(store openClawStore) []string {
	if list, ok := store.Order["openai-codex"]; ok {
		ordered := dedupeStrings(list)
		out := make([]string, 0, len(ordered))
		for _, id := range ordered {
			if id == openClawLegacyPendingLoginSentinelID {
				continue
			}
			out = append(out, id)
		}
		return out
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

func activeCredentialFromOpenClawEntry(entry openClawCredential) (Credential, bool) {
	if strings.ToLower(strings.TrimSpace(entry.Provider)) != "openai-codex" {
		return Credential{}, false
	}
	if entry.Type == "oauth" {
		if entry.Access == "" || entry.Refresh == "" {
			return Credential{}, false
		}
		return normalizeCredentialIdentity(Credential{
			Provider:  "openai-codex",
			Access:    entry.Access,
			Refresh:   entry.Refresh,
			Expires:   entry.Expires,
			AccountID: entry.AccountID,
			Email:     entry.Email,
		}), true
	}
	if entry.Type == "token" {
		if entry.Token == "" {
			return Credential{}, false
		}
		now := time.Now().UnixMilli()
		if entry.Expires > 0 && now >= entry.Expires {
			return Credential{}, false
		}
		return normalizeCredentialIdentity(Credential{
			Provider: "openai-codex",
			Access:   entry.Token,
			Expires:  entry.Expires,
			Email:    entry.Email,
		}), true
	}
	return Credential{}, false
}

func cleanupLegacyOpenClawPendingMarkers(store *openClawStore) {
	if store == nil {
		return
	}
	if store.Order == nil {
		store.Order = map[string][]string{}
	}
	if store.Raw == nil {
		store.Raw = map[string]any{}
	}

	if list, ok := store.Order["openai-codex"]; ok {
		next := make([]string, 0, len(list))
		for _, id := range list {
			if strings.TrimSpace(id) == "" {
				continue
			}
			if id == openClawLegacyPendingLoginSentinelID {
				continue
			}
			next = append(next, id)
		}
		store.Order["openai-codex"] = dedupeStrings(next)
	}

	delete(store.Raw, openClawLegacyPendingKnownIDsKey)
}

func removeLegacySwitcherOpenClawProfiles(store *openClawStore) {
	if store == nil || store.Profiles == nil {
		return
	}
	for id := range store.Profiles {
		if strings.HasPrefix(id, "openai-codex:rotater:") {
			delete(store.Profiles, id)
		}
	}
}

func countLegacySwitcherOpenClawProfiles(store openClawStore) int {
	count := 0
	for id := range store.Profiles {
		if strings.HasPrefix(id, "openai-codex:rotater:") {
			count++
		}
	}
	return count
}

func orderContainsID(order []string, id string) bool {
	for _, item := range order {
		if item == id {
			return true
		}
	}
	return false
}

func selectPreferredOpenClawCredentialEntry(store openClawStore) (openClawCredential, bool) {
	if entry, ok := store.Profiles[openClawManagedProfileID]; ok && openClawCredentialUsable(entry) {
		return normalizeOpenClawCredential(entry), true
	}

	for _, id := range activeOpenClawOrder(store) {
		entry, ok := store.Profiles[id]
		if !ok || !openClawCredentialUsable(entry) {
			continue
		}
		return normalizeOpenClawCredential(entry), true
	}

	ids := make([]string, 0, len(store.Profiles))
	for id := range store.Profiles {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		entry := store.Profiles[id]
		if !openClawCredentialUsable(entry) {
			continue
		}
		return normalizeOpenClawCredential(entry), true
	}

	return openClawCredential{}, false
}

func openClawCredentialUsable(entry openClawCredential) bool {
	if strings.ToLower(strings.TrimSpace(entry.Provider)) != "openai-codex" {
		return false
	}
	if entry.Type == "oauth" {
		return entry.Access != "" && entry.Refresh != ""
	}
	if entry.Type == "token" {
		if entry.Token == "" {
			return false
		}
		now := time.Now().UnixMilli()
		return entry.Expires <= 0 || now < entry.Expires
	}
	return false
}

func normalizeOpenClawCredential(entry openClawCredential) openClawCredential {
	entry.Provider = "openai-codex"
	if entry.Type == "oauth" && strings.TrimSpace(entry.ClientID) == "" {
		entry.ClientID = "app_EMoamEEZ73f0CkXaXp7hrann"
	}
	return entry
}

func stringSliceEqual(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
