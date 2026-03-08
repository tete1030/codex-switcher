package app

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
)

func credentialFingerprint(cred Credential) string {
	provider := strings.TrimSpace(cred.Provider)
	if provider == "" && cred.Access == "" && cred.Refresh == "" && cred.AccountID == "" {
		return ""
	}
	key := provider + "\x00" + cred.AccountID
	if cred.Refresh != "" {
		key += "\x00refresh\x00" + cred.Refresh
	} else {
		key += "\x00access\x00" + cred.Access
	}
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

func setActiveProfileTracking(state *StateFile, profile string, cred Credential) {
	if state == nil {
		return
	}
	state.ActiveProfile = profile
	state.ActiveCredentialHash = credentialFingerprint(cred)
}

func clearActiveProfileTracking(state *StateFile) {
	if state == nil {
		return
	}
	state.ActiveProfile = ""
	state.ActiveCredentialHash = ""
}

func profileMatchesCredential(paths ToolPaths, profile string, cred Credential) bool {
	if strings.TrimSpace(profile) == "" {
		return false
	}
	profileCred, err := loadProfile(paths, profile)
	if err != nil {
		return false
	}
	return credentialsLikelyMatch(cred, profileCred)
}

func verifiedStateActiveProfile(paths ToolPaths, state StateFile, active Credential) string {
	if strings.TrimSpace(state.ActiveProfile) == "" {
		return ""
	}
	activeHash := credentialFingerprint(active)
	if state.ActiveCredentialHash != "" {
		if activeHash != "" && state.ActiveCredentialHash == activeHash {
			return state.ActiveProfile
		}
		return ""
	}
	if matched := uniqueMatchingProfileName(paths, active); matched == state.ActiveProfile {
		return state.ActiveProfile
	}
	return ""
}

func activeProfileForDisplay(paths ToolPaths, adapter Adapter, state StateFile) string {
	active, ok, err := adapter.ReadActiveCredential(paths)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		return ""
	}
	if !ok {
		return ""
	}
	return verifiedStateActiveProfile(paths, state, active)
}

func uniqueMatchingProfileName(paths ToolPaths, active Credential) string {
	profiles, err := listProfiles(paths)
	if err != nil {
		return ""
	}
	matches := make([]string, 0, 1)
	for _, name := range profiles {
		if !profileMatchesCredential(paths, name, active) {
			continue
		}
		matches = append(matches, name)
		if len(matches) > 1 {
			return ""
		}
	}
	if len(matches) == 1 {
		return matches[0]
	}
	return ""
}
