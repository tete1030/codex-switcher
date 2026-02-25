# Managing Multiple OpenAI Accounts for AI Development Tools

This guide outlines the best practices for managing multiple OpenAI accounts (e.g., Work vs. Personal) across multiple computers for tools like **OpenCode**, **Codex** (CLI and macOS App), and **OpenClaw**.

## 1. How OpenAI Codex OAuth Works
When logging into these tools via OAuth, the system uses a standard PKCE (Proof Key for Code Exchange) flow.

* **Access Tokens:** Short-lived tokens (approx. 1 hour) that authenticate your active session.
* **Refresh Tokens:** Long-lived tokens stored locally on your machine.
* **Auto-Refresh:** The tools automatically use the refresh token to mint a new access token in the background before the current one expires. Manual refreshing is not required.

### ⚠️ The Cloud Sync Trap
**Never sync your auth configuration files across multiple computers using Dropbox, iCloud, or Google Drive.** When a tool refreshes an access token, the OAuth provider often issues a *new* refresh token and invalidates the old one. If Computer A refreshes the token, Computer B will attempt to use the newly invalidated token before the cloud sync catches up. This triggers a "Token Sink" error, invalidating your session and forcing a manual re-login on all devices. 

**Rule:** Authenticate each computer independently and manage account switching locally.

---

## 2. Configuration File Locations (macOS / Linux)

If you need to backup or manipulate your authentication state, these are the default paths:

* **Codex (CLI & macOS `Codex.app`)**
  * Auth File: `~/.codex/auth.json`
  * Config File: `~/.codex/config.toml`
* **OpenCode**
  * Auth File: `~/.local/share/opencode/auth.json`
* **OpenClaw**
  * Auth File/Config: `~/.openclaw/openclaw.json`
  * Agent Workspaces: `~/.openclaw/workspace-[agent-name]/`

---

## 3. Methods for Switching Accounts

### Method A: The Scripted Profile Swapper (Best for OAuth)
Since these tools rely on static file paths, you can back up your authenticated states (e.g., `auth-work.json`, `auth-personal.json`) and use shell aliases in your `~/.zshrc` or `~/.bashrc` to quickly swap them.

**Setup:**
1. Log into your Work account on all tools.
2. Copy the active `.json` files to `-work.json`.
3. Log into your Personal account on all tools.
4. Copy the active `.json` files to `-personal.json`.
5. Add the following to your shell profile:

```bash
# Switch to Work Profile
switch_ai_work() {
    cp ~/.codex/auth-work.json ~/.codex/auth.json
    cp ~/.local/share/opencode/auth-work.json ~/.local/share/opencode/auth.json
    cp ~/.openclaw/openclaw-work.json ~/.openclaw/openclaw.json
    echo "Switched AI tools to Work account."
}

# Switch to Personal Profile
switch_ai_personal() {
    cp ~/.codex/auth-personal.json ~/.codex/auth.json
    cp ~/.local/share/opencode/auth-personal.json ~/.local/share/opencode/auth.json
    cp ~/.openclaw/openclaw-personal.json ~/.openclaw/openclaw.json
    echo "Switched AI tools to Personal account."
}
```


