package app

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
)

func TestReleaseAssetNameForRuntime(t *testing.T) {
	cases := []struct {
		goos   string
		goarch string
		want   string
	}{
		{goos: "linux", goarch: "amd64", want: "codex-switcher-linux-x86_64"},
		{goos: "windows", goarch: "amd64", want: "codex-switcher-windows-x86_64.exe"},
		{goos: "darwin", goarch: "amd64", want: "codex-switcher-macos-x86_64"},
		{goos: "darwin", goarch: "arm64", want: "codex-switcher-macos-arm64"},
	}

	for _, tc := range cases {
		got, err := releaseAssetNameForRuntime(tc.goos, tc.goarch)
		if err != nil {
			t.Fatalf("unexpected error for %s/%s: %v", tc.goos, tc.goarch, err)
		}
		if got != tc.want {
			t.Fatalf("unexpected asset for %s/%s: got %q want %q", tc.goos, tc.goarch, got, tc.want)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	if compareVersions("v0.1.0", "v0.1.0") != 0 {
		t.Fatalf("expected equal versions")
	}
	if compareVersions("v0.1.0", "v0.2.0") >= 0 {
		t.Fatalf("expected older current version")
	}
	if compareVersions("v1.2.0", "v1.1.9") <= 0 {
		t.Fatalf("expected newer current version")
	}
}

func TestSelfUpdateCheckOnly(t *testing.T) {
	assetName, err := releaseAssetNameForRuntime(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Skipf("unsupported test runtime: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/tester/codex-switcher/releases/latest" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = fmt.Fprintf(w, `{"tag_name":"v9.9.9","html_url":"https://example/releases/v9.9.9","assets":[{"name":%q,"browser_download_url":"https://example/download"}]}`,
			assetName,
		)
	}))
	defer server.Close()

	prevVersion := Version
	Version = "v0.1.0"
	defer func() { Version = prevVersion }()

	t.Setenv("CODEX_SWITCHER_UPDATE_API_BASE", server.URL)

	svc := NewService()
	result, err := svc.SelfUpdate(SelfUpdateOptions{CheckOnly: true, Repo: "tester/codex-switcher"})
	if err != nil {
		t.Fatalf("self update check failed: %v", err)
	}
	if result.Status != "update_available" {
		t.Fatalf("expected update_available, got %q", result.Status)
	}
	if result.LatestVersion != "v9.9.9" {
		t.Fatalf("unexpected latest version: %q", result.LatestVersion)
	}
	if result.AssetName != assetName {
		t.Fatalf("unexpected asset name: %q", result.AssetName)
	}
}

func TestSelfUpdateCheckOnlyUpToDate(t *testing.T) {
	assetName, err := releaseAssetNameForRuntime(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Skipf("unsupported test runtime: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/tester/codex-switcher/releases/latest" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = fmt.Fprintf(w, `{"tag_name":"v0.1.0","html_url":"https://example/releases/v0.1.0","assets":[{"name":%q,"browser_download_url":"https://example/download"}]}`,
			assetName,
		)
	}))
	defer server.Close()

	prevVersion := Version
	Version = "v0.1.0"
	defer func() { Version = prevVersion }()

	t.Setenv("CODEX_SWITCHER_UPDATE_API_BASE", server.URL)

	svc := NewService()
	result, err := svc.SelfUpdate(SelfUpdateOptions{CheckOnly: true, Repo: "tester/codex-switcher"})
	if err != nil {
		t.Fatalf("self update check failed: %v", err)
	}
	if result.Status != "up_to_date" {
		t.Fatalf("expected up_to_date, got %q", result.Status)
	}
}

func TestRenameWithRetryReplacesExistingFile(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src.bin")
	dst := filepath.Join(tmp, "dst.bin")

	if err := os.WriteFile(src, []byte("new"), 0o600); err != nil {
		t.Fatalf("write src: %v", err)
	}
	if err := os.WriteFile(dst, []byte("old"), 0o600); err != nil {
		t.Fatalf("write dst: %v", err)
	}

	if err := renameWithRetry(src, dst, true); err != nil {
		t.Fatalf("renameWithRetry failed: %v", err)
	}

	bytes, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(bytes) != "new" {
		t.Fatalf("expected dst content from src, got %q", string(bytes))
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("expected src removed after rename, got err=%v", err)
	}
}

func TestIsRetryableRenameError(t *testing.T) {
	if !isRetryableRenameError(os.ErrPermission) {
		t.Fatalf("expected os.ErrPermission to be retryable")
	}

	wrapped := fmt.Errorf("wrapper: %w", os.ErrPermission)
	if !isRetryableRenameError(wrapped) {
		t.Fatalf("expected wrapped os.ErrPermission to be retryable")
	}

	if runtime.GOOS == "windows" {
		if !isRetryableRenameError(syscall.Errno(32)) {
			t.Fatalf("expected windows sharing violation to be retryable")
		}
		if isRetryableRenameError(syscall.Errno(2)) {
			t.Fatalf("did not expect file-not-found to be retryable")
		}
	} else {
		if !isRetryableRenameError(syscall.Errno(16)) {
			t.Fatalf("expected unix EBUSY to be retryable")
		}
		if isRetryableRenameError(syscall.Errno(2)) {
			t.Fatalf("did not expect ENOENT to be retryable")
		}
	}

	nonRetry := errors.New("invalid argument")
	if isRetryableRenameError(nonRetry) {
		t.Fatalf("expected non-retryable for plain error")
	}
}
