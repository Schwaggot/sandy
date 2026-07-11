package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func makeTarGz(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func makeZip(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func checksumLine(name string, data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]) + "  " + name + "\n"
}

// newTestUpdater serves the given release assets from an httptest server
// and returns an updater pointed at it and at a scratch binary path.
func newTestUpdater(t *testing.T, version string, assets map[string][]byte) *Updater {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/test/sandy/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"tag_name":"v%s"}`, version)
	})
	mux.HandleFunc("/test/sandy/releases/download/", func(w http.ResponseWriter, r *http.Request) {
		name := filepath.Base(r.URL.Path)
		data, ok := assets[name]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(data)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	execPath := filepath.Join(t.TempDir(), "sandy")
	if err := os.WriteFile(execPath, []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	return &Updater{
		Repo:     "test/sandy",
		APIBase:  srv.URL,
		DLBase:   srv.URL,
		ExecPath: execPath,
		OS:       "linux",
		Arch:     "amd64",
		Client:   srv.Client(),
	}
}

func TestRunReplacesBinary(t *testing.T) {
	newBin := []byte("new-binary")
	asset := "sandy_9.9.9_linux_amd64.tar.gz"
	archive := makeTarGz(t, "sandy", newBin)
	u := newTestUpdater(t, "9.9.9", map[string][]byte{
		asset:           archive,
		"checksums.txt": []byte(checksumLine(asset, archive)),
	})

	var out bytes.Buffer
	if err := u.Run("0.1.0", &out); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, err := os.ReadFile(u.ExecPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, newBin) {
		t.Errorf("binary not replaced: got %q", got)
	}
	info, err := os.Stat(u.ExecPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("mode: want 0755, got %v", info.Mode().Perm())
	}
	if !strings.Contains(out.String(), "0.1.0 -> 9.9.9") {
		t.Errorf("output missing version transition: %q", out.String())
	}
}

func TestRunAlreadyCurrent(t *testing.T) {
	// No assets registered: any download attempt would 404 and fail Run.
	u := newTestUpdater(t, "1.2.3", nil)
	var out bytes.Buffer
	if err := u.Run("1.2.3", &out); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, _ := os.ReadFile(u.ExecPath)
	if string(got) != "old-binary" {
		t.Errorf("binary must be untouched, got %q", got)
	}
}

func TestRunSkipsDevBuild(t *testing.T) {
	u := New()
	// Any network access would fail: point at a closed server.
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	u.APIBase = srv.URL
	u.DLBase = srv.URL

	var out bytes.Buffer
	if err := u.Run("dev", &out); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out.String(), "development build") {
		t.Errorf("expected dev-build skip message, got %q", out.String())
	}
}

func TestRunChecksumMismatch(t *testing.T) {
	asset := "sandy_9.9.9_linux_amd64.tar.gz"
	archive := makeTarGz(t, "sandy", []byte("new-binary"))
	bad := strings.Repeat("0", 64) + "  " + asset + "\n"
	u := newTestUpdater(t, "9.9.9", map[string][]byte{
		asset:           archive,
		"checksums.txt": []byte(bad),
	})

	err := u.Run("0.1.0", io.Discard)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("want checksum mismatch error, got %v", err)
	}
	got, _ := os.ReadFile(u.ExecPath)
	if string(got) != "old-binary" {
		t.Errorf("binary must be untouched on mismatch, got %q", got)
	}
}

func TestRunBinaryMissingFromArchive(t *testing.T) {
	asset := "sandy_9.9.9_linux_amd64.tar.gz"
	archive := makeTarGz(t, "README.md", []byte("docs only"))
	u := newTestUpdater(t, "9.9.9", map[string][]byte{
		asset:           archive,
		"checksums.txt": []byte(checksumLine(asset, archive)),
	})

	err := u.Run("0.1.0", io.Discard)
	if err == nil || !strings.Contains(err.Error(), "not found in") {
		t.Fatalf("want missing-binary error, got %v", err)
	}
}

func TestRunNeverDowngrades(t *testing.T) {
	// Current binary is ahead of the newest release (source build or
	// snapshot); no assets registered, so a download attempt would fail.
	u := newTestUpdater(t, "1.2.3", nil)
	for _, current := range []string{"2.0.0", "1.2.4-next", "v1.3.0"} {
		var out bytes.Buffer
		if err := u.Run(current, &out); err != nil {
			t.Fatalf("Run(%q): %v", current, err)
		}
		if !strings.Contains(out.String(), "up to date") {
			t.Errorf("Run(%q): expected up-to-date skip, got %q", current, out.String())
		}
	}
	got, _ := os.ReadFile(u.ExecPath)
	if string(got) != "old-binary" {
		t.Errorf("binary must be untouched, got %q", got)
	}
}

func TestNewerThan(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"0.4.0", "0.3.1", true},
		{"0.3.1", "0.3.1", false},
		{"0.3.1", "0.4.0", false},
		{"0.3.1", "0.3.2-next", false},
		{"1.0.0", "0.9.9", true},
		{"0.3.1", "v0.3.0", true},
		{"0.10.0", "0.9.0", true}, // numeric, not lexicographic
		{"abc", "0.3.1", true},    // unparseable falls back to inequality
		{"abc", "abc", false},
	}
	for _, c := range cases {
		if got := newerThan(c.latest, c.current); got != c.want {
			t.Errorf("newerThan(%q, %q): got %v, want %v", c.latest, c.current, got, c.want)
		}
	}
}

func TestExtractBinaryZip(t *testing.T) {
	content := []byte("exe-bytes")
	archive := makeZip(t, "sandy.exe", content)
	got, err := extractBinary(archive, "sandy_9.9.9_windows_amd64.zip", "windows")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("got %q, want %q", got, content)
	}
}

func TestAssetName(t *testing.T) {
	u := &Updater{OS: "darwin", Arch: "arm64"}
	if got := u.assetName("0.3.1"); got != "sandy_0.3.1_darwin_arm64.tar.gz" {
		t.Errorf("assetName: got %q", got)
	}
	u = &Updater{OS: "windows", Arch: "amd64"}
	if got := u.assetName("0.3.1"); got != "sandy_0.3.1_windows_amd64.zip" {
		t.Errorf("assetName: got %q", got)
	}
}
