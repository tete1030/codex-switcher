package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	toml "github.com/pelletier/go-toml/v2"
)

type codexAdapter struct{}

func (a *codexAdapter) Tool() ToolName { return ToolCodex }

type codexConfig struct {
	StoreMode string `toml:"cli_auth_credentials_store"`
}

func (a *codexAdapter) inspectStoreMode(paths ToolPaths) (mode string, blocked bool, reason string) {
	mode = "file"
	configPath := filepath.Join(paths.RootDir, "config.toml")
	bytes, err := os.ReadFile(configPath)
	if err != nil {
		return mode, false, ""
	}
	var cfg codexConfig
	if err := toml.Unmarshal(bytes, &cfg); err != nil {
		return mode, false, ""
	}
	if cfg.StoreMode != "" {
		mode = strings.ToLower(strings.TrimSpace(cfg.StoreMode))
	}

	switch mode {
	case "keyring":
		return mode, true, "codex is configured for keyring storage"
	case "ephemeral":
		return mode, true, "codex is configured for ephemeral storage"
	case "auto":
		cred, ok, _ := a.ReadActiveCredential(paths)
		if !ok || cred.Access == "" {
			return mode, true, "codex auto mode appears keyring-backed (no file tokens found)"
		}
	}

	return mode, false, ""
}

func (a *codexAdapter) Inspect(paths ToolPaths) (InspectToolResult, error) {
	out := InspectToolResult{
		Tool:  a.Tool(),
		Paths: paths,
	}

	mode, blocked, reason := a.inspectStoreMode(paths)
	out.StoreMode = mode
	out.SwitchBlocked = blocked
	out.SwitchBlockReason = reason

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

func (a *codexAdapter) ReadActiveCredential(paths ToolPaths) (Credential, bool, error) {
	bytes, err := os.ReadFile(paths.ActivePath)
	if err != nil {
		return Credential{}, false, err
	}

	var data map[string]any
	if err := json.Unmarshal(bytes, &data); err != nil {
		return Credential{}, false, err
	}
	tokensRaw, ok := data["tokens"].(map[string]any)
	if !ok {
		return Credential{}, false, nil
	}
	access, _ := tokensRaw["access_token"].(string)
	refresh, _ := tokensRaw["refresh_token"].(string)
	accountID, _ := tokensRaw["account_id"].(string)
	idToken, _ := tokensRaw["id_token"].(string)

	if access == "" || refresh == "" {
		return Credential{}, false, nil
	}

	cred := Credential{
		Provider:  "openai-codex",
		Access:    access,
		Refresh:   refresh,
		AccountID: accountID,
		IDToken:   idToken,
	}
	if idToken != "" {
		claims := parseJWTClaims(idToken)
		cred.Email = extractEmail(claims)
		if cred.AccountID == "" {
			cred.AccountID = extractAccountID(claims)
		}
	}
	return cred, true, nil
}

func (a *codexAdapter) WriteActiveCredential(paths ToolPaths, cred Credential) error {
	if cred.Access == "" || cred.Refresh == "" {
		return fmt.Errorf("codex credential requires access and refresh token")
	}

	var data map[string]any
	bytes, err := os.ReadFile(paths.ActivePath)
	if err == nil {
		_ = json.Unmarshal(bytes, &data)
	}
	if data == nil {
		data = map[string]any{}
	}

	tokensRaw, _ := data["tokens"].(map[string]any)
	if tokensRaw == nil {
		tokensRaw = map[string]any{}
	}
	tokensRaw["access_token"] = cred.Access
	tokensRaw["refresh_token"] = cred.Refresh
	if cred.AccountID != "" {
		tokensRaw["account_id"] = cred.AccountID
	} else {
		delete(tokensRaw, "account_id")
	}
	if cred.IDToken != "" {
		tokensRaw["id_token"] = cred.IDToken
	} else {
		delete(tokensRaw, "id_token")
	}

	data["auth_mode"] = "chatgpt"
	data["tokens"] = tokensRaw
	data["last_refresh"] = time.Now().UTC().Format(time.RFC3339)

	return writeJSONAtomic(paths.ActivePath, data)
}

func (a *codexAdapter) ClearActiveCredential(paths ToolPaths) error {
	var data map[string]any
	bytes, err := os.ReadFile(paths.ActivePath)
	if err == nil {
		_ = json.Unmarshal(bytes, &data)
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if data == nil {
		data = map[string]any{}
	}
	tokensRaw, _ := data["tokens"].(map[string]any)
	if tokensRaw == nil {
		tokensRaw = map[string]any{}
	}
	delete(tokensRaw, "access_token")
	delete(tokensRaw, "refresh_token")
	delete(tokensRaw, "account_id")
	delete(tokensRaw, "id_token")
	data["auth_mode"] = "chatgpt"
	data["tokens"] = tokensRaw
	data["last_refresh"] = time.Now().UTC().Format(time.RFC3339)
	return writeJSONAtomic(paths.ActivePath, data)
}
