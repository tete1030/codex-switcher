package app

type StatusToolResult struct {
	Tool                 ToolName  `json:"tool"`
	Paths                ToolPaths `json:"paths"`
	HasActive            bool      `json:"hasActive"`
	StoreMode            string    `json:"storeMode,omitempty"`
	SwitchBlocked        bool      `json:"switchBlocked,omitempty"`
	SwitchBlockReason    string    `json:"switchBlockReason,omitempty"`
	ActiveProfile        string    `json:"activeProfile,omitempty"`
	PreviousProfile      string    `json:"previousProfile,omitempty"`
	PendingCreateProfile string    `json:"pendingCreateProfile,omitempty"`
	PendingCreateSince   string    `json:"pendingCreateSince,omitempty"`
	ProfileCount         int       `json:"profileCount"`
	Profiles             []string  `json:"profiles,omitempty"`
}

func (s *Service) Status(tools []ToolName) ([]StatusToolResult, error) {
	results := make([]StatusToolResult, 0, len(tools))
	for _, tool := range tools {
		paths, err := resolveToolPaths(tool)
		if err != nil {
			return nil, err
		}
		adapter := adapterFor(tool)
		if adapter == nil {
			continue
		}
		inspect, err := adapter.Inspect(paths)
		if err != nil {
			return nil, err
		}
		state, err := loadState(paths)
		if err != nil {
			return nil, err
		}
		profiles, err := listProfiles(paths)
		if err != nil {
			return nil, err
		}

		results = append(results, StatusToolResult{
			Tool:                 tool,
			Paths:                paths,
			HasActive:            inspect.HasActive,
			StoreMode:            inspect.StoreMode,
			SwitchBlocked:        inspect.SwitchBlocked,
			SwitchBlockReason:    inspect.SwitchBlockReason,
			ActiveProfile:        state.ActiveProfile,
			PreviousProfile:      state.PreviousProfile,
			PendingCreateProfile: state.PendingCreateProfile,
			PendingCreateSince:   state.PendingCreateSince,
			ProfileCount:         len(profiles),
			Profiles:             profiles,
		})
	}
	return results, nil
}
