package app

import (
	"encoding/json"
	"fmt"
	"os"
)

type openCodeAdapter struct{}

func (a *openCodeAdapter) Tool() ToolName { return ToolOpenCode }

func (a *openCodeAdapter) Inspect(paths ToolPaths) (InspectToolResult, error) {
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

func (a *openCodeAdapter) ReadActiveCredential(paths ToolPaths) (Credential, bool, error) {
	bytes, err := os.ReadFile(paths.ActivePath)
	if err != nil {
		return Credential{}, false, err
	}
	var root map[string]any
	if err := json.Unmarshal(bytes, &root); err != nil {
		return Credential{}, false, err
	}
	openAI, ok := root["openai"].(map[string]any)
	if !ok {
		return Credential{}, false, nil
	}
	typeName, _ := openAI["type"].(string)
	if typeName != "oauth" {
		return Credential{}, false, nil
	}
	access, _ := openAI["access"].(string)
	refresh, _ := openAI["refresh"].(string)
	if access == "" || refresh == "" {
		return Credential{}, false, nil
	}

	expires := toInt64(openAI["expires"])
	accountID, _ := openAI["accountId"].(string)

	cred := Credential{
		Provider:  "openai-codex",
		Access:    access,
		Refresh:   refresh,
		Expires:   expires,
		AccountID: accountID,
	}
	return cred, true, nil
}

func (a *openCodeAdapter) WriteActiveCredential(paths ToolPaths, cred Credential) error {
	if cred.Access == "" || cred.Refresh == "" {
		return fmt.Errorf("opencode credential requires access and refresh token")
	}

	var root map[string]any
	bytes, err := os.ReadFile(paths.ActivePath)
	if err == nil {
		_ = json.Unmarshal(bytes, &root)
	}
	if root == nil {
		root = map[string]any{}
	}

	entry := map[string]any{
		"type":    "oauth",
		"access":  cred.Access,
		"refresh": cred.Refresh,
	}
	if cred.Expires > 0 {
		entry["expires"] = cred.Expires
	}
	if cred.AccountID != "" {
		entry["accountId"] = cred.AccountID
	}
	root["openai"] = entry

	return writeJSONAtomic(paths.ActivePath, root)
}

func (a *openCodeAdapter) ClearActiveCredential(paths ToolPaths) error {
	var root map[string]any
	bytes, err := os.ReadFile(paths.ActivePath)
	if err == nil {
		_ = json.Unmarshal(bytes, &root)
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if root == nil {
		root = map[string]any{}
	}
	delete(root, "openai")
	return writeJSONAtomic(paths.ActivePath, root)
}
