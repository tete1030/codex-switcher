package app

import "time"

type ToolName string

const (
	ToolCodex    ToolName = "codex"
	ToolOpenCode ToolName = "opencode"
	ToolOpenClaw ToolName = "openclaw"
)

var AllTools = []ToolName{ToolCodex, ToolOpenCode, ToolOpenClaw}

type Credential struct {
	Provider  string `json:"provider"`
	Access    string `json:"access"`
	Refresh   string `json:"refresh"`
	Expires   int64  `json:"expires,omitempty"`
	AccountID string `json:"accountId,omitempty"`
	IDToken   string `json:"idToken,omitempty"`
	Email     string `json:"email,omitempty"`
	UpdatedAt int64  `json:"updatedAt,omitempty"`
}

func (c Credential) IsExpired(now time.Time) bool {
	if c.Expires <= 0 {
		return false
	}
	return now.UnixMilli() >= c.Expires
}

func (c Credential) NearExpiry(now time.Time, threshold time.Duration) bool {
	if c.Expires <= 0 {
		return false
	}
	return now.Add(threshold).UnixMilli() >= c.Expires
}

type ProfileFile struct {
	Version  int    `json:"version"`
	Provider string `json:"provider"`
	Access   string `json:"access"`
	Refresh  string `json:"refresh"`

	Expires   int64  `json:"expires,omitempty"`
	AccountID string `json:"accountId,omitempty"`
	IDToken   string `json:"idToken,omitempty"`
	Email     string `json:"email,omitempty"`
	UpdatedAt int64  `json:"updatedAt,omitempty"`
}

type StateFile struct {
	Version              int    `json:"version"`
	ActiveProfile        string `json:"activeProfile,omitempty"`
	PreviousProfile      string `json:"previousProfile,omitempty"`
	LastSwitchAt         string `json:"lastSwitchAt,omitempty"`
	PendingCreateProfile string `json:"pendingCreateProfile,omitempty"`
	PendingCreateSince   string `json:"pendingCreateSince,omitempty"`
}

type ToolPaths struct {
	Tool       ToolName `json:"tool"`
	RootDir    string   `json:"rootDir"`
	ActivePath string   `json:"activePath"`
	ProfileDir string   `json:"profileDir"`
	StatePath  string   `json:"statePath"`
	LockPath   string   `json:"lockPath"`
}

type InspectToolResult struct {
	Tool              ToolName  `json:"tool"`
	Paths             ToolPaths `json:"paths"`
	HasActive         bool      `json:"hasActive"`
	Capturable        bool      `json:"capturable"`
	AccountID         string    `json:"accountId,omitempty"`
	Email             string    `json:"email,omitempty"`
	Expires           int64     `json:"expires,omitempty"`
	StoreMode         string    `json:"storeMode,omitempty"`
	SwitchBlocked     bool      `json:"switchBlocked,omitempty"`
	SwitchBlockReason string    `json:"switchBlockReason,omitempty"`
	Warnings          []string  `json:"warnings,omitempty"`
}

type UsageWindow struct {
	Label         string  `json:"label"`
	UsedPercent   float64 `json:"usedPercent"`
	ResetAt       int64   `json:"resetAt,omitempty"`
	ResetAtISO    string  `json:"resetAtIso,omitempty"`
	RemainingDays float64 `json:"remainingDays,omitempty"`
}

type UsageResult struct {
	Tool           ToolName      `json:"tool,omitempty"`
	Profile        string        `json:"profile"`
	Provider       string        `json:"provider"`
	AccountID      string        `json:"accountId,omitempty"`
	Plan           string        `json:"plan,omitempty"`
	CreditsBalance *float64      `json:"creditsBalance,omitempty"`
	Windows        []UsageWindow `json:"windows"`
	Status         string        `json:"status"`
	Error          string        `json:"error,omitempty"`
	Refreshed      bool          `json:"refreshed,omitempty"`
}
