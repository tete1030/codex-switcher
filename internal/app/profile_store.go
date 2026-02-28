package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const profilePrefix = "openai-codex."
const profileSuffix = ".json"

func profilePath(paths ToolPaths, name string) string {
	return filepath.Join(paths.ProfileDir, profilePrefix+name+profileSuffix)
}

func listProfiles(paths ToolPaths) ([]string, error) {
	entries, err := os.ReadDir(paths.ProfileDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	profiles := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, profilePrefix) || !strings.HasSuffix(name, profileSuffix) {
			continue
		}
		core := strings.TrimSuffix(strings.TrimPrefix(name, profilePrefix), profileSuffix)
		if core != "" {
			profiles = append(profiles, core)
		}
	}
	sort.Strings(profiles)
	return profiles, nil
}

func loadProfile(paths ToolPaths, name string) (Credential, error) {
	if err := validateProfileName(name); err != nil {
		return Credential{}, err
	}
	var p ProfileFile
	if err := readJSONFile(profilePath(paths, name), &p); err != nil {
		return Credential{}, err
	}
	if p.Provider != "openai-codex" {
		return Credential{}, fmt.Errorf("profile %q has unsupported provider %q", name, p.Provider)
	}
	if p.Access == "" || p.Refresh == "" {
		return Credential{}, fmt.Errorf("profile %q is missing access/refresh token", name)
	}
	cred := Credential{
		Provider:  p.Provider,
		Access:    p.Access,
		Refresh:   p.Refresh,
		Expires:   p.Expires,
		AccountID: p.AccountID,
		IDToken:   p.IDToken,
		Email:     p.Email,
		UpdatedAt: p.UpdatedAt,
	}
	return normalizeCredentialIdentity(cred), nil
}

func saveProfile(paths ToolPaths, name string, cred Credential, force bool) error {
	if err := validateProfileName(name); err != nil {
		return err
	}
	if cred.Provider == "" {
		cred.Provider = "openai-codex"
	}
	if cred.Provider != "openai-codex" {
		return fmt.Errorf("unsupported provider %q", cred.Provider)
	}
	if cred.Access == "" || cred.Refresh == "" {
		return fmt.Errorf("credential is missing access/refresh token")
	}

	path := profilePath(paths, name)
	if !force {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("profile %q already exists for %s (use --force to overwrite)", name, paths.Tool)
		}
	}

	p := ProfileFile{
		Version:   1,
		Provider:  "openai-codex",
		Access:    cred.Access,
		Refresh:   cred.Refresh,
		Expires:   cred.Expires,
		AccountID: cred.AccountID,
		IDToken:   cred.IDToken,
		Email:     cred.Email,
		UpdatedAt: time.Now().UnixMilli(),
	}
	return writeJSONAtomic(path, p)
}

func deleteProfile(paths ToolPaths, name string) error {
	if err := validateProfileName(name); err != nil {
		return err
	}
	err := os.Remove(profilePath(paths, name))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
