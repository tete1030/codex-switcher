package app

import (
	"os"
	"time"
)

func loadState(paths ToolPaths) (StateFile, error) {
	var s StateFile
	err := readJSONFile(paths.StatePath, &s)
	if err != nil {
		if os.IsNotExist(err) {
			return StateFile{Version: 1}, nil
		}
		return StateFile{}, err
	}
	if s.Version == 0 {
		s.Version = 1
	}
	return s, nil
}

func saveState(paths ToolPaths, state StateFile) error {
	state.Version = 1
	if state.LastSwitchAt == "" {
		state.LastSwitchAt = time.Now().UTC().Format(time.RFC3339)
	}
	return writeJSONAtomic(paths.StatePath, state)
}
