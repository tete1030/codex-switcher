package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func ensureParentDir(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o700)
}

func writeJSONAtomic(path string, value any) error {
	if err := ensureParentDir(path); err != nil {
		return err
	}

	bytes, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	bytes = append(bytes, '\n')

	if err := writeFileAtomic(path, bytes, 0o600); err != nil {
		return err
	}
	return nil
}

func writeFileAtomic(path string, content []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	tmp := filepath.Join(dir, fmt.Sprintf(".%s.tmp.%d", base, time.Now().UnixNano()))

	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer func() {
		_ = os.Remove(tmp)
	}()

	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}

	if err := os.Rename(tmp, path); err != nil {
		return err
	}

	dirFD, err := os.Open(dir)
	if err != nil {
		return nil
	}
	defer dirFD.Close()
	_ = dirFD.Sync()
	return nil
}

func readJSONFile(path string, out any) error {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(bytes, out); err != nil {
		return err
	}
	return nil
}
