package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	renameRetries     = 20
	renameRetryDelay  = 150 * time.Millisecond
	renameRetryFactor = 2
)

type SelfUpdateOptions struct {
	CheckOnly bool
	Force     bool
	Repo      string
}

type SelfUpdateResult struct {
	Repo           string `json:"repo"`
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	ReleaseURL     string `json:"releaseUrl,omitempty"`
	AssetName      string `json:"assetName,omitempty"`
	Status         string `json:"status"`
	Path           string `json:"path,omitempty"`
	Message        string `json:"message,omitempty"`
}

type githubReleaseResponse struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func (s *Service) SelfUpdate(opts SelfUpdateOptions) (SelfUpdateResult, error) {
	repo := strings.TrimSpace(opts.Repo)
	if repo == "" {
		repo = strings.TrimSpace(os.Getenv("CODEX_SWITCHER_UPDATE_REPO"))
	}
	if repo == "" {
		repo = defaultUpdateRepo
	}

	owner, name, err := splitRepo(repo)
	if err != nil {
		return SelfUpdateResult{}, WrapExit(ExitUserError, err)
	}

	assetName, err := releaseAssetNameForRuntime(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return SelfUpdateResult{}, WrapExit(ExitUserError, err)
	}

	release, err := fetchLatestRelease(owner, name)
	if err != nil {
		return SelfUpdateResult{}, WrapExit(ExitIOFailure, err)
	}

	assetURL := ""
	for _, asset := range release.Assets {
		if asset.Name == assetName {
			assetURL = asset.BrowserDownloadURL
			break
		}
	}
	if assetURL == "" {
		return SelfUpdateResult{}, WrapExit(ExitIOFailure, fmt.Errorf("release %s does not include asset %s", release.TagName, assetName))
	}

	result := SelfUpdateResult{
		Repo:           owner + "/" + name,
		CurrentVersion: Version,
		LatestVersion:  release.TagName,
		ReleaseURL:     release.HTMLURL,
		AssetName:      assetName,
	}

	if !opts.Force {
		cmp := compareVersions(Version, release.TagName)
		if cmp >= 0 {
			result.Status = "up_to_date"
			result.Message = "current version is already up to date"
			return result, nil
		}
	}

	if opts.CheckOnly {
		result.Status = "update_available"
		result.Message = "new release is available"
		return result, nil
	}

	execPath, err := os.Executable()
	if err != nil {
		return SelfUpdateResult{}, WrapExit(ExitIOFailure, err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		execPath = filepath.Clean(execPath)
	}

	mode := os.FileMode(0o755)
	if stat, statErr := os.Stat(execPath); statErr == nil {
		mode = stat.Mode() & os.ModePerm
		if mode == 0 {
			mode = 0o755
		}
	}

	if runtime.GOOS == "windows" {
		target := execPath + ".new"
		if strings.HasSuffix(strings.ToLower(execPath), ".exe") {
			target = strings.TrimSuffix(execPath, ".exe") + ".new.exe"
		}
		if err := downloadFile(assetURL, target, mode); err != nil {
			return SelfUpdateResult{}, WrapExit(ExitIOFailure, err)
		}
		result.Status = "downloaded"
		result.Path = target
		result.Message = "downloaded update next to current executable; replace binary after process exits"
		return result, nil
	}

	tmpPath := filepath.Join(filepath.Dir(execPath), fmt.Sprintf(".%s.update.%d", filepath.Base(execPath), time.Now().UnixNano()))
	if err := downloadFile(assetURL, tmpPath, mode); err != nil {
		_ = os.Remove(tmpPath)
		return SelfUpdateResult{}, WrapExit(ExitIOFailure, err)
	}

	if err := renameWithRetry(tmpPath, execPath, true); err != nil {
		_ = os.Remove(tmpPath)
		return SelfUpdateResult{}, WrapExit(ExitIOFailure, err)
	}

	result.Status = "updated"
	result.Path = execPath
	result.Message = "updated executable in place"
	return result, nil
}

func splitRepo(repo string) (string, string, error) {
	parts := strings.Split(strings.TrimSpace(repo), "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", fmt.Errorf("invalid repo %q (expected owner/repo)", repo)
	}
	return parts[0], parts[1], nil
}

func releaseAssetNameForRuntime(goos string, goarch string) (string, error) {
	switch goos {
	case "linux":
		if goarch == "amd64" {
			return "codex-switcher-linux-x86_64", nil
		}
	case "windows":
		if goarch == "amd64" {
			return "codex-switcher-windows-x86_64.exe", nil
		}
	case "darwin":
		if goarch == "amd64" {
			return "codex-switcher-macos-x86_64", nil
		}
		if goarch == "arm64" {
			return "codex-switcher-macos-arm64", nil
		}
	}
	return "", fmt.Errorf("unsupported runtime for self-update: %s/%s", goos, goarch)
}

func fetchLatestRelease(owner string, repo string) (githubReleaseResponse, error) {
	apiBase := strings.TrimRight(strings.TrimSpace(os.Getenv("CODEX_SWITCHER_UPDATE_API_BASE")), "/")
	if apiBase == "" {
		apiBase = "https://api.github.com"
	}
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", apiBase, owner, repo)

	client := &http.Client{Timeout: 20 * time.Second}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return githubReleaseResponse{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "codex-switcher/"+Version)

	res, err := client.Do(req)
	if err != nil {
		return githubReleaseResponse{}, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 4*1024*1024))
	if res.StatusCode < 200 || res.StatusCode > 299 {
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = http.StatusText(res.StatusCode)
		}
		return githubReleaseResponse{}, fmt.Errorf("failed to fetch latest release (%d): %s", res.StatusCode, msg)
	}

	var payload githubReleaseResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return githubReleaseResponse{}, err
	}
	if strings.TrimSpace(payload.TagName) == "" {
		return githubReleaseResponse{}, errors.New("latest release has empty tag_name")
	}
	return payload, nil
}

func downloadFile(url string, path string, mode os.FileMode) error {
	client := &http.Client{Timeout: 5 * time.Minute}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "codex-switcher/"+Version)

	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 8192))
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = http.StatusText(res.StatusCode)
		}
		return fmt.Errorf("download failed (%d): %s", res.StatusCode, msg)
	}

	if err := ensureParentDir(path); err != nil {
		return err
	}

	tmpPath := fmt.Sprintf("%s.part.%d", path, time.Now().UnixNano())
	file, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(file, res.Body); err != nil {
		_ = file.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	if err := os.Chmod(tmpPath, mode); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	if err := renameWithRetry(tmpPath, path, true); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func renameWithRetry(src string, dst string, replaceExisting bool) error {
	delay := renameRetryDelay
	var lastErr error

	for attempt := 0; attempt < renameRetries; attempt++ {
		if replaceExisting {
			if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
				if isRetryableRenameError(err) {
					lastErr = err
					time.Sleep(delay)
					delay = nextRetryDelay(delay)
					continue
				}
				return err
			}
		}

		err := os.Rename(src, dst)
		if err == nil {
			return nil
		}
		if !isRetryableRenameError(err) {
			return err
		}
		lastErr = err
		time.Sleep(delay)
		delay = nextRetryDelay(delay)
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("rename failed without explicit error")
	}
	return fmt.Errorf("failed to rename %s -> %s after retries (file may be temporarily locked): %w", src, dst, lastErr)
}

func nextRetryDelay(current time.Duration) time.Duration {
	next := current * renameRetryFactor
	if next > 2*time.Second {
		return 2 * time.Second
	}
	return next
}

func isRetryableRenameError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrPermission) {
		return true
	}

	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return false
	}

	if runtime.GOOS == "windows" {
		switch uint32(errno) {
		case 5, 32, 33, 1224:
			return true
		default:
			return false
		}
	}

	switch uint32(errno) {
	case 1, 13, 16, 26:
		return true
	default:
		return false
	}
}

func compareVersions(current string, latest string) int {
	cur, okCur := parseSemver(current)
	lat, okLat := parseSemver(latest)
	if !okCur || !okLat {
		return -1
	}
	for i := 0; i < 3; i++ {
		if cur[i] < lat[i] {
			return -1
		}
		if cur[i] > lat[i] {
			return 1
		}
	}
	return 0
}

func parseSemver(value string) ([3]int, bool) {
	clean := strings.TrimSpace(strings.TrimPrefix(value, "v"))
	if clean == "" {
		return [3]int{}, false
	}
	if idx := strings.Index(clean, "-"); idx >= 0 {
		clean = clean[:idx]
	}
	parts := strings.Split(clean, ".")
	if len(parts) != 3 {
		return [3]int{}, false
	}
	out := [3]int{}
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return [3]int{}, false
		}
		out[i] = n
	}
	return out, true
}
