package store

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const (
	stDownloadBaseAMD64 = "https://download.thimeo.com/stereo_tool_gui_jack_64"
	stDownloadBaseARM64 = "https://download.thimeo.com/stereo_tool_gui_jack_pi2_64"

	// stDownloadTimeout caps the download; Stereo Tool binaries are ~10 MB.
	stDownloadTimeout = 5 * time.Minute
)

// StereoToolArch returns "arm64" or "amd64" — the server's architecture,
// which determines which Thimeo binary URL to use.
func StereoToolArch() string {
	return runtime.GOARCH
}

// StereoToolDownloadURL returns the Thimeo download URL for the current
// architecture. version may be "" (latest) or a version string like "1030".
func StereoToolDownloadURL(version string) string {
	base := stDownloadBaseAMD64
	if runtime.GOARCH == "arm64" {
		base = stDownloadBaseARM64
	}
	if version == "" {
		return base
	}
	return fmt.Sprintf("%s_%s", base, version)
}

// StereoToolInstalled reports whether a file exists at installPath.
func StereoToolInstalled(installPath string) bool {
	_, err := os.Stat(installPath)
	return err == nil
}

// InstallStereoTool downloads the Stereo Tool binary from Thimeo and
// installs it to installPath. version may be "" (latest) or a version
// string like "1030". The existing binary is replaced atomically via a
// temp file + rename in the same directory, so a partial download never
// leaves a broken binary in place.
func InstallStereoTool(installPath, version string) error {
	url := StereoToolDownloadURL(version)

	client := &http.Client{Timeout: stDownloadTimeout}
	resp, err := client.Get(url) //nolint:noctx
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned HTTP %d from %s", resp.StatusCode, url)
	}

	dir := filepath.Dir(installPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("cannot create install directory %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, ".stereo_tool_download_*")
	if err != nil {
		return fmt.Errorf("cannot create temp file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	// Always remove the temp file; os.Remove on a path that no longer exists
	// (because Rename succeeded) is a no-op on Linux.
	defer os.Remove(tmpPath)

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		return fmt.Errorf("write failed: %w", err)
	}
	tmp.Close()

	if err := os.Chmod(tmpPath, 0o755); err != nil {
		return fmt.Errorf("chmod failed: %w", err)
	}

	if err := os.Rename(tmpPath, installPath); err != nil {
		return fmt.Errorf("install to %s failed: %w", installPath, err)
	}
	return nil
}
