package app

import "testing"

func TestResolveToolPathsOpenClawDefaultUsesHome(t *testing.T) {
	t.Setenv("HOME", "/tmp/home-default")
	t.Setenv("USERPROFILE", "")
	t.Setenv("OPENCLAW_HOME", "")
	t.Setenv("OPENCLAW_STATE_DIR", "")
	t.Setenv("CLAWDBOT_STATE_DIR", "")
	t.Setenv("OPENCLAW_AGENT_DIR", "")
	t.Setenv("PI_CODING_AGENT_DIR", "")

	paths, err := resolveToolPaths(ToolOpenClaw)
	if err != nil {
		t.Fatalf("resolveToolPaths failed: %v", err)
	}

	want := "/tmp/home-default/.openclaw/agents/main/agent"
	if paths.RootDir != want {
		t.Fatalf("unexpected root dir: got %q, want %q", paths.RootDir, want)
	}
}

func TestResolveToolPathsOpenClawUsesStateDirOverride(t *testing.T) {
	t.Setenv("HOME", "/tmp/home-state")
	t.Setenv("USERPROFILE", "")
	t.Setenv("OPENCLAW_HOME", "")
	t.Setenv("OPENCLAW_STATE_DIR", "")
	t.Setenv("CLAWDBOT_STATE_DIR", "/tmp/custom-state")
	t.Setenv("OPENCLAW_AGENT_DIR", "")
	t.Setenv("PI_CODING_AGENT_DIR", "")

	paths, err := resolveToolPaths(ToolOpenClaw)
	if err != nil {
		t.Fatalf("resolveToolPaths failed: %v", err)
	}

	want := "/tmp/custom-state/agents/main/agent"
	if paths.RootDir != want {
		t.Fatalf("unexpected root dir: got %q, want %q", paths.RootDir, want)
	}
}

func TestResolveToolPathsOpenClawUsesOpenClawHomeForTildeState(t *testing.T) {
	t.Setenv("HOME", "/tmp/home-ignored")
	t.Setenv("USERPROFILE", "")
	t.Setenv("OPENCLAW_HOME", "/tmp/openclaw-home")
	t.Setenv("OPENCLAW_STATE_DIR", "~/state")
	t.Setenv("CLAWDBOT_STATE_DIR", "")
	t.Setenv("OPENCLAW_AGENT_DIR", "")
	t.Setenv("PI_CODING_AGENT_DIR", "")

	paths, err := resolveToolPaths(ToolOpenClaw)
	if err != nil {
		t.Fatalf("resolveToolPaths failed: %v", err)
	}

	want := "/tmp/openclaw-home/state/agents/main/agent"
	if paths.RootDir != want {
		t.Fatalf("unexpected root dir: got %q, want %q", paths.RootDir, want)
	}
}

func TestResolveToolPathsOpenClawAgentDirOverridesState(t *testing.T) {
	t.Setenv("HOME", "/tmp/home-agent")
	t.Setenv("USERPROFILE", "")
	t.Setenv("OPENCLAW_HOME", "")
	t.Setenv("OPENCLAW_STATE_DIR", "/tmp/custom-state")
	t.Setenv("CLAWDBOT_STATE_DIR", "")
	t.Setenv("OPENCLAW_AGENT_DIR", "/tmp/direct-agent")
	t.Setenv("PI_CODING_AGENT_DIR", "")

	paths, err := resolveToolPaths(ToolOpenClaw)
	if err != nil {
		t.Fatalf("resolveToolPaths failed: %v", err)
	}

	want := "/tmp/direct-agent"
	if paths.RootDir != want {
		t.Fatalf("unexpected root dir: got %q, want %q", paths.RootDir, want)
	}
}
