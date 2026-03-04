package updater

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	releaseURL    = "https://api.github.com/repos/getmac-io/getmac-gitlab-executor/releases/latest"
	cooldownFile  = ".getmac-gitlab-executor/last-update-check"
	cooldownHours = 24
)

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// CheckAndUpdate checks GitHub for a newer release and replaces the current binary if found.
// Errors are returned but callers should treat them as non-fatal.
func CheckAndUpdate(currentVersion string) error {
	if currentVersion == "dev" {
		return nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	tsPath := filepath.Join(homeDir, cooldownFile)
	if !cooldownExpired(tsPath) {
		return nil
	}

	resp, err := http.Get(releaseURL)
	if err != nil {
		return fmt.Errorf("fetch latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("github api returned status %d", resp.StatusCode)
	}

	var release ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return fmt.Errorf("decode release: %w", err)
	}

	// Write cooldown timestamp after successful API call
	writeCooldown(tsPath)

	latestVersion := strings.TrimPrefix(release.TagName, "v")
	if !isNewer(latestVersion, currentVersion) {
		return nil
	}

	assetName := fmt.Sprintf("getmac-gitlab-executor-%s-%s-%s.tar.gz",
		release.TagName, runtime.GOOS, runtime.GOARCH)

	var downloadURL string
	for _, a := range release.Assets {
		if a.Name == assetName {
			downloadURL = a.BrowserDownloadURL
			break
		}
	}
	if downloadURL == "" {
		return fmt.Errorf("no asset found matching %s", assetName)
	}

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("resolve symlinks: %w", err)
	}

	if err := downloadAndReplace(downloadURL, execPath); err != nil {
		return fmt.Errorf("update binary: %w", err)
	}

	fmt.Fprintf(os.Stderr, "getmac-gitlab-executor updated to %s\n", release.TagName)
	return nil
}

func cooldownExpired(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return true
	}
	ts, err := time.Parse(time.RFC3339, strings.TrimSpace(string(data)))
	if err != nil {
		return true
	}
	return time.Since(ts) >= cooldownHours*time.Hour
}

func writeCooldown(path string) {
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, []byte(time.Now().UTC().Format(time.RFC3339)), 0o644)
}

func isNewer(latest, current string) bool {
	lp := parseVersion(latest)
	cp := parseVersion(current)
	for i := 0; i < 3; i++ {
		if lp[i] > cp[i] {
			return true
		}
		if lp[i] < cp[i] {
			return false
		}
	}
	return false
}

func parseVersion(v string) [3]int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)
	var result [3]int
	for i, p := range parts {
		if i >= 3 {
			break
		}
		result[i], _ = strconv.Atoi(p)
	}
	return result
}

func downloadAndReplace(url, execPath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("binary not found in archive")
		}
		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}

		if hdr.Typeflag != tar.TypeReg {
			continue
		}

		baseName := filepath.Base(hdr.Name)
		if baseName != "getmac-gitlab-executor" {
			continue
		}

		// Write to temp file in same directory for atomic rename
		dir := filepath.Dir(execPath)
		tmp, err := os.CreateTemp(dir, ".getmac-update-*")
		if err != nil {
			return fmt.Errorf("create temp file: %w", err)
		}
		tmpPath := tmp.Name()

		if _, err := io.Copy(tmp, tr); err != nil {
			tmp.Close()
			os.Remove(tmpPath)
			return fmt.Errorf("write temp file: %w", err)
		}
		tmp.Close()

		if err := os.Chmod(tmpPath, 0o755); err != nil {
			os.Remove(tmpPath)
			return err
		}

		if err := os.Rename(tmpPath, execPath); err != nil {
			os.Remove(tmpPath)
			return err
		}

		return nil
	}
}
