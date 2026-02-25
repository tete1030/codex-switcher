package app

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	lockWaitTimeout = 10 * time.Second
	lockRetryDelay  = 100 * time.Millisecond
	lockStaleAfter  = 30 * time.Second
)

type FileLock struct {
	path string
}

func acquireLock(path string) (*FileLock, error) {
	if err := ensureParentDir(path); err != nil {
		return nil, err
	}

	deadline := time.Now().Add(lockWaitTimeout)
	for {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_, _ = file.WriteString(fmt.Sprintf("%d\n%d\n", os.Getpid(), time.Now().Unix()))
			_ = file.Close()
			return &FileLock{path: path}, nil
		}

		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}

		stale, staleErr := isStaleLock(path)
		if staleErr == nil && stale {
			_ = os.Remove(path)
			continue
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timeout acquiring lock %s", path)
		}
		time.Sleep(lockRetryDelay)
	}
}

func isStaleLock(path string) (bool, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	parts := strings.Split(strings.TrimSpace(string(bytes)), "\n")
	if len(parts) < 2 {
		return true, nil
	}
	ts, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return true, nil
	}
	created := time.Unix(ts, 0)
	return time.Since(created) > lockStaleAfter, nil
}

func (l *FileLock) Release() error {
	if l == nil {
		return nil
	}
	if err := os.Remove(l.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
