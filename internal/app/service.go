package app

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

type Service struct{}

type SwitchOptions struct {
	DryRun        bool
	CreateMissing bool
}

type rollbackRecord struct {
	tool       ToolName
	paths      ToolPaths
	activeRaw  []byte
	activeSeen bool
	stateRaw   []byte
	stateSeen  bool
}

func NewService() *Service {
	return &Service{}
}

func ParseTools(raw string) ([]ToolName, error) {
	if strings.TrimSpace(raw) == "" {
		return append([]ToolName{}, AllTools...), nil
	}
	parts := strings.Split(raw, ",")
	tools := make([]ToolName, 0, len(parts))
	seen := map[ToolName]struct{}{}
	for _, part := range parts {
		name := ToolName(strings.ToLower(strings.TrimSpace(part)))
		if name == "" {
			continue
		}
		if name != ToolCodex && name != ToolOpenCode && name != ToolOpenClaw {
			return nil, fmt.Errorf("unknown tool %q", name)
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		tools = append(tools, name)
	}
	if len(tools) == 0 {
		return nil, fmt.Errorf("no tools selected")
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i] < tools[j] })
	return tools, nil
}

func (s *Service) Inspect(tools []ToolName) ([]InspectToolResult, error) {
	results := make([]InspectToolResult, 0, len(tools))
	for _, tool := range tools {
		adapter := adapterFor(tool)
		if adapter == nil {
			return nil, fmt.Errorf("no adapter for %s", tool)
		}
		paths, err := resolveToolPaths(tool)
		if err != nil {
			return nil, err
		}
		result, err := adapter.Inspect(paths)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func (s *Service) Capture(profile string, tools []ToolName, force bool) ([]InspectToolResult, error) {
	if err := validateProfileName(profile); err != nil {
		return nil, WrapExit(ExitUserError, err)
	}

	results := make([]InspectToolResult, 0, len(tools))
	for _, tool := range tools {
		adapter := adapterFor(tool)
		paths, err := resolveToolPaths(tool)
		if err != nil {
			return nil, WrapExit(ExitIOFailure, err)
		}

		lock, err := acquireLock(paths.LockPath)
		if err != nil {
			return nil, WrapExit(ExitIOFailure, err)
		}
		releaseLock := func() {
			_ = lock.Release()
		}

		cred, ok, err := adapter.ReadActiveCredential(paths)
		if err != nil {
			releaseLock()
			if os.IsNotExist(err) {
				results = append(results, InspectToolResult{Tool: tool, Paths: paths, HasActive: false})
				continue
			}
			return nil, WrapExit(ExitIOFailure, err)
		}
		if !ok {
			releaseLock()
			results = append(results, InspectToolResult{Tool: tool, Paths: paths, HasActive: false})
			continue
		}

		if err := saveProfile(paths, profile, cred, force); err != nil {
			releaseLock()
			return nil, WrapExit(ExitUserError, err)
		}
		state, err := loadState(paths)
		if err != nil {
			releaseLock()
			return nil, WrapExit(ExitIOFailure, err)
		}
		if state.ActiveProfile != profile {
			state.PreviousProfile = state.ActiveProfile
		}
		state.ActiveProfile = profile
		state.PendingCreateProfile = ""
		state.PendingCreateSince = ""
		state.LastSwitchAt = time.Now().UTC().Format(time.RFC3339)
		if err := saveState(paths, state); err != nil {
			releaseLock()
			return nil, WrapExit(ExitIOFailure, err)
		}
		releaseLock()

		results = append(results, InspectToolResult{
			Tool:       tool,
			Paths:      paths,
			HasActive:  true,
			Capturable: true,
			AccountID:  cred.AccountID,
			Expires:    cred.Expires,
			Email:      cred.Email,
		})
	}
	return results, nil
}

type SwitchResult struct {
	Tool            ToolName `json:"tool"`
	FromProfile     string   `json:"fromProfile,omitempty"`
	ToProfile       string   `json:"toProfile"`
	SnapshotProfile string   `json:"snapshotProfile,omitempty"`
	Changed         bool     `json:"changed"`
	Status          string   `json:"status"`
	Warning         string   `json:"warning,omitempty"`
	PendingCreate   bool     `json:"pendingCreate,omitempty"`
}

func (s *Service) Switch(profile string, tools []ToolName, opts SwitchOptions) ([]SwitchResult, error) {
	if err := validateProfileName(profile); err != nil {
		return nil, WrapExit(ExitUserError, err)
	}

	type target struct {
		tool        ToolName
		paths       ToolPaths
		adapter     Adapter
		state       StateFile
		action      string
		cred        Credential
		materialize bool
	}

	targets := make([]target, 0, len(tools))
	results := make([]SwitchResult, 0, len(tools))
	for _, tool := range tools {
		adapter := adapterFor(tool)
		if adapter == nil {
			return nil, WrapExit(ExitUserError, fmt.Errorf("unknown tool %s", tool))
		}
		paths, err := resolveToolPaths(tool)
		if err != nil {
			return nil, WrapExit(ExitIOFailure, err)
		}
		inspect, err := adapter.Inspect(paths)
		if err != nil {
			return nil, WrapExit(ExitIOFailure, err)
		}
		state, err := loadState(paths)
		if err != nil {
			return nil, WrapExit(ExitIOFailure, err)
		}
		if inspect.SwitchBlocked {
			results = append(results, SwitchResult{
				Tool:      tool,
				ToProfile: profile,
				Status:    "blocked",
				Warning:   inspect.SwitchBlockReason,
			})
			continue
		}
		cred, err := loadProfile(paths, profile)
		if err != nil {
			if os.IsNotExist(err) {
				activeCred, hasActiveCred, activeErr := adapter.ReadActiveCredential(paths)
				if activeErr != nil && !os.IsNotExist(activeErr) {
					return nil, WrapExit(ExitIOFailure, activeErr)
				}
				canMaterialize := hasActiveCred && activeCred.Access != "" && activeCred.Refresh != ""
				if canMaterialize && state.PendingCreateProfile == profile {
					targets = append(targets, target{tool: tool, paths: paths, adapter: adapter, state: state, action: "switch", cred: activeCred, materialize: true})
					continue
				}
				if opts.CreateMissing {
					targets = append(targets, target{tool: tool, paths: paths, adapter: adapter, state: state, action: "prepare"})
				} else {
					results = append(results, SwitchResult{
						Tool:      tool,
						ToProfile: profile,
						Status:    "skipped_missing",
						Warning:   "profile does not exist for this tool (use --create to prepare login)",
					})
				}
				continue
			}
			return nil, WrapExit(ExitUserError, fmt.Errorf("%s: %w", tool, err))
		}
		targets = append(targets, target{tool: tool, paths: paths, adapter: adapter, cred: cred, state: state, action: "switch"})
	}

	if opts.DryRun {
		for _, t := range targets {
			status := "switched"
			pending := false
			changed := true
			snapshotProfile := chooseSnapshotProfile(t.state, profile)
			if t.action == "prepare" {
				status = "prepared"
				pending = true
			} else if t.action == "switch" && !t.materialize && t.state.ActiveProfile == profile {
				status = "already_active"
				changed = false
				snapshotProfile = ""
			}
			results = append(results, SwitchResult{
				Tool:            t.tool,
				FromProfile:     t.state.ActiveProfile,
				ToProfile:       profile,
				SnapshotProfile: snapshotProfile,
				Changed:         changed,
				Status:          status,
				PendingCreate:   pending,
			})
		}
		sortSwitchResults(results)
		return results, nil
	}

	if len(targets) == 0 {
		sortSwitchResults(results)
		return results, nil
	}

	locks := make([]*FileLock, 0, len(targets))
	for _, t := range targets {
		lock, err := acquireLock(t.paths.LockPath)
		if err != nil {
			for _, held := range locks {
				_ = held.Release()
			}
			return nil, WrapExit(ExitIOFailure, err)
		}
		locks = append(locks, lock)
	}
	defer func() {
		for _, lock := range locks {
			_ = lock.Release()
		}
	}()

	rollback := []rollbackRecord{}

	for _, t := range targets {
		activeRaw, activeErr := os.ReadFile(t.paths.ActivePath)
		activeSeen := activeErr == nil
		if activeErr != nil && !os.IsNotExist(activeErr) {
			return nil, WrapExit(ExitIOFailure, activeErr)
		}
		stateRaw, stateErr := os.ReadFile(t.paths.StatePath)
		stateSeen := stateErr == nil
		if stateErr != nil && !os.IsNotExist(stateErr) {
			return nil, WrapExit(ExitIOFailure, stateErr)
		}

		oldCred, hadCred, err := t.adapter.ReadActiveCredential(t.paths)
		if err != nil && !os.IsNotExist(err) {
			return nil, WrapExit(ExitIOFailure, err)
		}

		oldState := t.state
		rollback = append(rollback, rollbackRecord{
			tool: t.tool, paths: t.paths,
			activeRaw: activeRaw, activeSeen: activeSeen,
			stateRaw: stateRaw, stateSeen: stateSeen,
		})

		sameActiveTarget := t.action == "switch" && !t.materialize && oldState.ActiveProfile == profile
		if sameActiveTarget {
			status := "already_active"
			changed := false

			if hadCred && oldCred.Refresh != "" && oldCred.Access != "" {
				if err := saveProfile(t.paths, profile, oldCred, true); err != nil {
					s.rollback(rollback)
					return nil, WrapExit(ExitIOFailure, err)
				}
			} else {
				status = "switched"
				changed = true
				if t.tool == ToolOpenClaw {
					oa, ok := t.adapter.(*openClawAdapter)
					if !ok {
						s.rollback(rollback)
						return nil, WrapExit(ExitIOFailure, errors.New("openclaw adapter mismatch"))
					}
					if err := oa.WriteWithProfile(t.paths, profile, t.cred); err != nil {
						s.rollback(rollback)
						return nil, WrapExit(ExitIOFailure, err)
					}
				} else {
					if err := t.adapter.WriteActiveCredential(t.paths, t.cred); err != nil {
						s.rollback(rollback)
						return nil, WrapExit(ExitIOFailure, err)
					}
				}
			}

			newState := oldState
			newState.Version = 1
			newState.ActiveProfile = profile
			newState.PendingCreateProfile = ""
			newState.PendingCreateSince = ""
			newState.LastSwitchAt = time.Now().UTC().Format(time.RFC3339)
			if err := saveState(t.paths, newState); err != nil {
				s.rollback(rollback)
				return nil, WrapExit(ExitIOFailure, err)
			}

			results = append(results, SwitchResult{
				Tool:            t.tool,
				FromProfile:     oldState.ActiveProfile,
				ToProfile:       profile,
				SnapshotProfile: "",
				Changed:         changed,
				Status:          status,
				PendingCreate:   false,
			})
			continue
		}

		snapshotProfile := chooseSnapshotProfile(oldState, profile)
		if hadCred && oldCred.Refresh != "" && oldCred.Access != "" {
			if err := saveProfile(t.paths, snapshotProfile, oldCred, true); err != nil {
				s.rollback(rollback)
				return nil, WrapExit(ExitIOFailure, err)
			}
		}

		if t.materialize {
			if err := saveProfile(t.paths, profile, t.cred, true); err != nil {
				s.rollback(rollback)
				return nil, WrapExit(ExitIOFailure, err)
			}
		}

		status := "switched"
		pendingCreate := false
		if t.action == "switch" {
			if t.tool == ToolOpenClaw {
				oa, ok := t.adapter.(*openClawAdapter)
				if !ok {
					s.rollback(rollback)
					return nil, WrapExit(ExitIOFailure, errors.New("openclaw adapter mismatch"))
				}
				if err := oa.WriteWithProfile(t.paths, profile, t.cred); err != nil {
					s.rollback(rollback)
					return nil, WrapExit(ExitIOFailure, err)
				}
			} else {
				if err := t.adapter.WriteActiveCredential(t.paths, t.cred); err != nil {
					s.rollback(rollback)
					return nil, WrapExit(ExitIOFailure, err)
				}
			}
		} else {
			status = "prepared"
			pendingCreate = true
			if err := t.adapter.ClearActiveCredential(t.paths); err != nil {
				s.rollback(rollback)
				return nil, WrapExit(ExitIOFailure, err)
			}
		}

		newState := StateFile{
			Version:         1,
			PreviousProfile: snapshotProfile,
			LastSwitchAt:    time.Now().UTC().Format(time.RFC3339),
		}
		if t.action == "switch" {
			newState.ActiveProfile = profile
			newState.PendingCreateProfile = ""
			newState.PendingCreateSince = ""
		} else {
			newState.ActiveProfile = ""
			newState.PendingCreateProfile = profile
			newState.PendingCreateSince = time.Now().UTC().Format(time.RFC3339)
		}
		if err := saveState(t.paths, newState); err != nil {
			s.rollback(rollback)
			return nil, WrapExit(ExitIOFailure, err)
		}

		results = append(results, SwitchResult{
			Tool:            t.tool,
			FromProfile:     oldState.ActiveProfile,
			ToProfile:       profile,
			SnapshotProfile: snapshotProfile,
			Changed:         true,
			Status:          status,
			PendingCreate:   pendingCreate,
		})
	}

	sortSwitchResults(results)
	return results, nil
}

func sortSwitchResults(results []SwitchResult) {
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].Tool < results[j].Tool
	})
}

func chooseSnapshotProfile(state StateFile, target string) string {
	if state.ActiveProfile != "" && state.ActiveProfile != target {
		return state.ActiveProfile
	}
	return "__last__"
}

func (s *Service) rollback(records []rollbackRecord) {
	for i := len(records) - 1; i >= 0; i-- {
		record := records[i]
		if record.activeSeen {
			_ = writeFileAtomic(record.paths.ActivePath, record.activeRaw, 0o600)
		} else {
			_ = os.Remove(record.paths.ActivePath)
		}
		if record.stateSeen {
			_ = writeFileAtomic(record.paths.StatePath, record.stateRaw, 0o600)
		} else {
			_ = os.Remove(record.paths.StatePath)
		}
	}
}

func (s *Service) ListProfiles(tool ToolName) ([]string, error) {
	paths, err := resolveToolPaths(tool)
	if err != nil {
		return nil, err
	}
	return listProfiles(paths)
}

func (s *Service) DeleteProfile(name string, tools []ToolName) error {
	if err := validateProfileName(name); err != nil {
		return WrapExit(ExitUserError, err)
	}
	for _, tool := range tools {
		paths, err := resolveToolPaths(tool)
		if err != nil {
			return WrapExit(ExitIOFailure, err)
		}

		lock, err := acquireLock(paths.LockPath)
		if err != nil {
			return WrapExit(ExitIOFailure, err)
		}

		if err := deleteProfile(paths, name); err != nil {
			_ = lock.Release()
			return WrapExit(ExitIOFailure, err)
		}

		if tool == ToolOpenClaw {
			if err := removeOpenClawRotaterProfile(paths, name); err != nil {
				_ = lock.Release()
				return WrapExit(ExitIOFailure, err)
			}
		}

		state, err := loadState(paths)
		if err != nil {
			_ = lock.Release()
			return WrapExit(ExitIOFailure, err)
		}
		changed := false
		if state.ActiveProfile == name {
			state.ActiveProfile = ""
			changed = true
		}
		if state.PreviousProfile == name {
			state.PreviousProfile = ""
			changed = true
		}
		if state.PendingCreateProfile == name {
			state.PendingCreateProfile = ""
			state.PendingCreateSince = ""
			changed = true
		}
		if changed {
			state.LastSwitchAt = time.Now().UTC().Format(time.RFC3339)
			if err := saveState(paths, state); err != nil {
				_ = lock.Release()
				return WrapExit(ExitIOFailure, err)
			}
		}

		if err := lock.Release(); err != nil {
			return WrapExit(ExitIOFailure, err)
		}
	}
	return nil
}
