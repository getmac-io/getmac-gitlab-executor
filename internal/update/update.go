// Package update reports when a newer release of the executor is available.
//
// It deliberately never modifies the binary. The executor runs as a GitLab
// Runner custom executor: once per job stage, on machines we do not control.
// Replacing the binary from here would swap versions between the stages of a
// single job (config on the old build, cleanup on the new one) and would make
// every runner execute code fetched at job time. Reporting is the half that
// helps without either risk.
//
// Nothing here can fail a job. Every error path ends in silence, and the whole
// check is bounded by a short timeout.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	// latestReleaseAPI is the unauthenticated GitHub endpoint for the newest
	// release. Unauthenticated callers get 60 requests per hour per IP, which a
	// busy runner would exhaust in minutes without the cache below.
	latestReleaseAPI = "https://api.github.com/repos/getmac-io/getmac-gitlab-executor/releases/latest"

	// ReleasesPage is what we point the operator at; it needs no API call.
	ReleasesPage = "https://github.com/getmac-io/getmac-gitlab-executor/releases/latest"

	// checkInterval is how often we ask GitHub. In between, the warning is
	// re-emitted from cache, so every job log still shows it.
	checkInterval = 24 * time.Hour

	// httpTimeout bounds the config stage's exposure to a slow or hung GitHub.
	// The config stage blocks the job starting, so this stays small.
	httpTimeout = 3 * time.Second

	stateDirName  = "getmac-gitlab-executor"
	stateFileName = "update-check.json"
)

// devVersion is what the linker leaves in place for unreleased builds.
const devVersion = "dev"

type release struct {
	TagName string `json:"tag_name"`
}

// state persists across invocations so the executor can warn on every job while
// only querying GitHub once per checkInterval.
type state struct {
	// CheckedAt records the last *attempt*, not the last success. Recording
	// failures too is what stops a broken or rate-limited API from being
	// re-queried on every single job.
	CheckedAt time.Time `json:"checked_at"`
	LatestTag string    `json:"latest_tag"`
}

// Check logs a warning when a newer release exists. It never returns an error:
// an update check must not be able to break someone's pipeline.
func Check(ctx context.Context, current string) {
	// An unstamped or development build has no meaningful version to compare,
	// and nagging a developer running a local build is noise.
	if current == "" || current == devVersion {
		return
	}

	path := statePath()
	st, _ := loadState(path)

	if time.Since(st.CheckedAt) >= checkInterval {
		tag, err := fetchLatestTag(ctx)
		// Record the attempt either way; on failure we keep whatever tag we
		// already knew and simply report from it.
		st.CheckedAt = time.Now().UTC()
		if err == nil {
			st.LatestTag = tag
		}
		saveState(path, st)
	}

	if st.LatestTag == "" || !isNewer(st.LatestTag, current) {
		return
	}

	slog.Warn("A newer version of getmac-gitlab-executor is available",
		"current", normalize(current),
		"latest", normalize(st.LatestTag),
		"upgrade", ReleasesPage,
	)
}

func fetchLatestTag(ctx context.Context) (string, error) {
	return fetchTagFrom(ctx, latestReleaseAPI)
}

// fetchTagFrom takes the endpoint as an argument so tests can point it at a
// local server instead of reaching GitHub.
func fetchTagFrom(ctx context.Context, url string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, httpTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	// GitHub rejects API requests without a User-Agent.
	req.Header.Set("User-Agent", "getmac-gitlab-executor")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github api returned status %d", resp.StatusCode)
	}

	var r release
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", err
	}
	if r.TagName == "" {
		return "", fmt.Errorf("release has no tag_name")
	}

	return r.TagName, nil
}

// statePath prefers the user cache directory and falls back to the temp dir, so
// a runner running as a service account without HOME still gets a cache rather
// than querying GitHub on every job.
func statePath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}

	return filepath.Join(dir, stateDirName, stateFileName)
}

func loadState(path string) (state, error) {
	var st state

	data, err := os.ReadFile(path)
	if err != nil {
		return st, err
	}

	if err := json.Unmarshal(data, &st); err != nil {
		return state{}, err
	}

	return st, nil
}

func saveState(path string, st state) {
	data, err := json.Marshal(st)
	if err != nil {
		return
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}

	// Written via a temp file so a concurrent reader never sees a partial
	// object; several job stages can run at once on a busy runner.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".update-check-*")
	if err != nil {
		return
	}

	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return
	}

	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return
	}

	if err := os.Chmod(tmpPath, 0o600); err != nil {
		os.Remove(tmpPath)
		return
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
	}
}

// isNewer reports whether latest is a strictly higher version than current.
// Both may carry a leading "v" and a pre-release or build suffix.
func isNewer(latest, current string) bool {
	l, lok := parseVersion(latest)
	c, cok := parseVersion(current)
	if !lok || !cok {
		return false
	}

	for i := range l {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}

	return false
}

// parseVersion turns "v1.2.3", "1.2", or "v1.2.3-rc1" into numeric components.
// The bool reports whether the string looked like a version at all; an
// unparseable value must not be treated as 0.0.0, or "dev" would look older
// than every release and warn forever.
func parseVersion(v string) ([3]int, bool) {
	var out [3]int

	v = normalize(v)
	// Drop any pre-release ("-rc1") or build ("+meta") suffix.
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	if v == "" {
		return out, false
	}

	for i, part := range strings.SplitN(v, ".", 3) {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return [3]int{}, false
		}
		out[i] = n
	}

	return out, true
}

func normalize(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "v")
}
