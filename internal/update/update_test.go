package update

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		in   string
		want [3]int
		ok   bool
	}{
		{"v1.2.3", [3]int{1, 2, 3}, true},
		{"1.2.3", [3]int{1, 2, 3}, true},
		{" v0.1.0 ", [3]int{0, 1, 0}, true},
		{"v1.2", [3]int{1, 2, 0}, true},
		{"v2", [3]int{2, 0, 0}, true},
		{"v1.2.3-rc1", [3]int{1, 2, 3}, true},
		{"v1.2.3+build7", [3]int{1, 2, 3}, true},
		// "dev" must not parse, or an unstamped build compares as 0.0.0 and
		// warns against every release forever.
		{"dev", [3]int{}, false},
		{"", [3]int{}, false},
		{"v1.x.3", [3]int{}, false},
	}

	for _, tt := range tests {
		got, ok := parseVersion(tt.in)
		if ok != tt.ok {
			t.Errorf("parseVersion(%q) ok = %v, want %v", tt.in, ok, tt.ok)
			continue
		}
		if ok && got != tt.want {
			t.Errorf("parseVersion(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestIsNewer(t *testing.T) {
	tests := []struct {
		latest, current string
		want            bool
	}{
		{"v0.1.0", "v0.0.4", true},
		{"v1.0.0", "v0.9.9", true},
		{"v0.0.5", "v0.0.4", true},
		{"v0.1.0", "v0.1.0", false},
		{"v0.0.4", "v0.1.0", false},
		// Real tags from this repo's history.
		{"v0.1.0", "v0.0.3", true},
		// A 10 must beat a 9; string comparison would get this wrong.
		{"v0.10.0", "v0.9.0", true},
		// Unparseable either side means no warning.
		{"v0.1.0", "dev", false},
		{"garbage", "v0.1.0", false},
	}

	for _, tt := range tests {
		if got := isNewer(tt.latest, tt.current); got != tt.want {
			t.Errorf("isNewer(%q, %q) = %v, want %v", tt.latest, tt.current, got, tt.want)
		}
	}
}

func TestStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "update-check.json")

	want := state{CheckedAt: time.Now().UTC().Truncate(time.Second), LatestTag: "v0.1.0"}
	saveState(path, want)

	got, err := loadState(path)
	if err != nil {
		t.Fatalf("loadState: %v", err)
	}
	if got.LatestTag != want.LatestTag {
		t.Errorf("LatestTag = %q, want %q", got.LatestTag, want.LatestTag)
	}
	if !got.CheckedAt.Equal(want.CheckedAt) {
		t.Errorf("CheckedAt = %v, want %v", got.CheckedAt, want.CheckedAt)
	}
}

func TestLoadStateMissingFile(t *testing.T) {
	st, err := loadState(filepath.Join(t.TempDir(), "absent.json"))
	if err == nil {
		t.Error("expected an error for a missing state file")
	}
	// A missing file must yield a zero CheckedAt so the first run checks.
	if !st.CheckedAt.IsZero() {
		t.Errorf("CheckedAt = %v, want zero", st.CheckedAt)
	}
}

func TestFetchLatestTag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request sent without a User-Agent; GitHub rejects those")
		}
		json.NewEncoder(w).Encode(release{TagName: "v0.1.0"})
	}))
	defer srv.Close()

	tag, err := fetchTagFrom(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("fetchTagFrom: %v", err)
	}
	if tag != "v0.1.0" {
		t.Errorf("tag = %q, want v0.1.0", tag)
	}
}

func TestFetchLatestTagNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// What GitHub returns once an unauthenticated runner is rate limited.
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	if _, err := fetchTagFrom(context.Background(), srv.URL); err == nil {
		t.Error("expected an error for a non-200 response")
	}
}

func TestCheckDoesNotPanicOnDevBuild(t *testing.T) {
	// Point the cache somewhere writable and assert the dev short-circuit means
	// no state file is ever created (i.e. no network call was attempted).
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)

	Check(context.Background(), "dev")

	if _, err := os.Stat(filepath.Join(dir, stateDirName, stateFileName)); err == nil {
		t.Error("dev build wrote a state file; it should short-circuit before checking")
	}
}
