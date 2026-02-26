package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func resolveToolPaths(tool ToolName) (ToolPaths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return ToolPaths{}, err
	}

	switch tool {
	case ToolCodex:
		root := resolvePathWithHome(firstNonEmpty(os.Getenv("CODEX_HOME"), filepath.Join(home, ".codex")), home)
		return ToolPaths{
			Tool:       tool,
			RootDir:    root,
			ActivePath: filepath.Join(root, "auth.json"),
			ProfileDir: filepath.Join(root, "profiles"),
			StatePath:  filepath.Join(root, "profiles", ".rotater-state.json"),
			LockPath:   filepath.Join(root, "profiles", ".rotater.lock"),
		}, nil
	case ToolOpenCode:
		xdgData := firstNonEmpty(os.Getenv("XDG_DATA_HOME"), filepath.Join(home, ".local", "share"))
		root := filepath.Join(resolvePathWithHome(xdgData, home), "opencode")
		return ToolPaths{
			Tool:       tool,
			RootDir:    root,
			ActivePath: filepath.Join(root, "auth.json"),
			ProfileDir: filepath.Join(root, "profiles"),
			StatePath:  filepath.Join(root, "profiles", ".rotater-state.json"),
			LockPath:   filepath.Join(root, "profiles", ".rotater.lock"),
		}, nil
	case ToolOpenClaw:
		openClawHome := resolveOpenClawHome(home)
		openClawStateDir := resolveOpenClawStateDir(openClawHome)
		agentDir := firstNonEmpty(
			strings.TrimSpace(os.Getenv("OPENCLAW_AGENT_DIR")),
			strings.TrimSpace(os.Getenv("PI_CODING_AGENT_DIR")),
			filepath.Join(openClawStateDir, "agents", "main", "agent"),
		)
		root := resolvePathWithHome(agentDir, openClawHome)
		return ToolPaths{
			Tool:       tool,
			RootDir:    root,
			ActivePath: filepath.Join(root, "auth-profiles.json"),
			ProfileDir: filepath.Join(root, "profiles"),
			StatePath:  filepath.Join(root, "profiles", ".rotater-state.json"),
			LockPath:   filepath.Join(root, "profiles", ".rotater.lock"),
		}, nil
	default:
		return ToolPaths{}, errors.New("unsupported tool")
	}
}

func resolveOpenClawHome(fallbackHome string) string {
	return resolvePathWithHome(
		firstNonEmpty(
			strings.TrimSpace(os.Getenv("OPENCLAW_HOME")),
			strings.TrimSpace(os.Getenv("HOME")),
			strings.TrimSpace(os.Getenv("USERPROFILE")),
			fallbackHome,
		),
		fallbackHome,
	)
}

func resolveOpenClawStateDir(openClawHome string) string {
	stateOverride := firstNonEmpty(
		strings.TrimSpace(os.Getenv("OPENCLAW_STATE_DIR")),
		strings.TrimSpace(os.Getenv("CLAWDBOT_STATE_DIR")),
	)
	if stateOverride != "" {
		return resolvePathWithHome(stateOverride, openClawHome)
	}
	return filepath.Join(openClawHome, ".openclaw")
}

func resolvePathWithHome(raw string, home string) string {
	if strings.HasPrefix(raw, "~/") {
		return filepath.Join(home, strings.TrimPrefix(raw, "~/"))
	}
	if strings.HasPrefix(raw, "~\\") {
		return filepath.Join(home, strings.TrimPrefix(raw, "~\\"))
	}
	if raw == "~" {
		return home
	}
	if filepath.IsAbs(raw) {
		return filepath.Clean(raw)
	}
	return filepath.Clean(raw)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		trimmed := strings.TrimSpace(v)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
