package updatestatus

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 2, 18, 0, 0, 0, time.UTC)

func plant(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "updater.json")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

const failing = `{"result":"failing","code":"fetch-auth","checkedAt":"2026-10-02T17:58:00Z","since":"2026-09-22T09:14:02Z","deployed":"v1.38.0","latest":""}` + "\n"

// Nothing to say: the feature is off, or the host has no status file.
func TestReadSaysNothingWithoutAPathOrAFile(t *testing.T) {
	if s, ok := Read("", now); ok || s != (Status{}) {
		t.Errorf("empty path: got %+v ok=%v, want the zero value and false", s, ok)
	}
	if s, ok := Read(filepath.Join(t.TempDir(), "updater.json"), now); ok || s != (Status{}) {
		t.Errorf("missing file: got %+v ok=%v, want the zero value and false", s, ok)
	}
}

func TestReadAValidFile(t *testing.T) {
	s, ok := Read(plant(t, failing), now)
	want := Status{State: "failing", Code: "fetch-auth", CheckedAt: "2026-10-02T17:58:00Z", Since: "2026-09-22T09:14:02Z", Deployed: "v1.38.0"}
	if !ok || s != want {
		t.Fatalf("got %+v ok=%v, want %+v", s, ok, want)
	}
	okBody := `{"result":"ok","code":"ok","checkedAt":"2026-10-02T17:58:00Z","since":"","deployed":"v1.40.0","latest":"v1.40.0"}`
	s, ok = Read(plant(t, okBody), now)
	want = Status{State: "ok", Code: "ok", CheckedAt: "2026-10-02T17:58:00Z", Deployed: "v1.40.0", Latest: "v1.40.0"}
	if !ok || s != want {
		t.Fatalf("got %+v ok=%v, want %+v", s, ok, want)
	}
}

// A status that stopped being refreshed is stale — strictly after StaleAfter.
func TestReadStaleBoundary(t *testing.T) {
	body := func(checked time.Time) string {
		return `{"result":"ok","code":"ok","checkedAt":"` + checked.Format(time.RFC3339) + `","since":"","deployed":"v1.40.0","latest":"v1.40.0"}`
	}
	if s, _ := Read(plant(t, body(now.Add(-StaleAfter))), now); s.State != "ok" {
		t.Errorf("exactly StaleAfter old: state %q, want ok", s.State)
	}
	s, ok := Read(plant(t, body(now.Add(-StaleAfter-time.Second))), now)
	if !ok || s.State != "stale" || s.Deployed != "v1.40.0" || s.CheckedAt == "" {
		t.Errorf("one second past StaleAfter: got %+v ok=%v, want stale with its fields kept", s, ok)
	}
	// A failing status that went stale is stale too: nobody is checking any more.
	old := strings.Replace(failing, "2026-10-02T17:58:00Z", "2026-10-02T10:00:00Z", 1)
	if s, _ := Read(plant(t, old), now); s.State != "stale" || s.Code != "fetch-auth" {
		t.Errorf("an old failing status: got %+v, want stale with its code", s)
	}
}

// A file that is there and cannot be trusted is "unknown" — never "ok", and
// with no field of the file passed through.
func TestReadUntrustedFilesAreUnknown(t *testing.T) {
	cases := map[string]string{
		"not JSON":                  `updater ok`,
		"empty":                     ``,
		"an extra key":              `{"result":"ok","code":"ok","checkedAt":"2026-10-02T17:58:00Z","since":"","deployed":"","latest":"","message":"hi"}`,
		"a code outside the set":    `{"result":"failing","code":"<script>","checkedAt":"2026-10-02T17:58:00Z","since":"2026-10-02T17:58:00Z","deployed":"","latest":""}`,
		"an ok with a failing code": `{"result":"ok","code":"fetch","checkedAt":"2026-10-02T17:58:00Z","since":"","deployed":"","latest":""}`,
		"an unknown result":         `{"result":"maybe","code":"ok","checkedAt":"2026-10-02T17:58:00Z","since":"","deployed":"","latest":""}`,
		"a tag that is not a tag":   `{"result":"ok","code":"ok","checkedAt":"2026-10-02T17:58:00Z","since":"","deployed":"v1.40.0; rm -rf /","latest":""}`,
		"a bad latest":              `{"result":"ok","code":"ok","checkedAt":"2026-10-02T17:58:00Z","since":"","deployed":"","latest":"main"}`,
		"a bad checkedAt":           `{"result":"ok","code":"ok","checkedAt":"yesterday","since":"","deployed":"","latest":""}`,
		"failing without a since":   `{"result":"failing","code":"fetch","checkedAt":"2026-10-02T17:58:00Z","since":"","deployed":"","latest":""}`,
		"two values":                failing + failing,
		"over the size bound":       `{"result":"ok","code":"ok","checkedAt":"2026-10-02T17:58:00Z","since":"","deployed":"","latest":""}` + strings.Repeat(" ", maxFile),
	}
	for name, body := range cases {
		s, ok := Read(plant(t, body), now)
		if !ok || s != (Status{State: "unknown"}) {
			t.Errorf("%s: got %+v ok=%v, want exactly {State: unknown} and true", name, s, ok)
		}
	}
	// A directory in the file's place: it exists and cannot be read as a file.
	dir := filepath.Join(t.TempDir(), "updater.json")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if s, ok := Read(dir, now); !ok || s != (Status{State: "unknown"}) {
		t.Errorf("a directory: got %+v ok=%v, want unknown and true", s, ok)
	}
}
