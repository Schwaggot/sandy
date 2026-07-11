// Package selfupdate replaces the running sandy binary with the latest
// GitHub release, verifying the download against the release checksums.
package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
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

// Updater checks GitHub releases for a newer sandy binary and replaces
// the current executable in place. Fields exist so tests can point the
// updater at a fake server and a scratch binary.
type Updater struct {
	Repo     string // owner/name
	APIBase  string // GitHub API base URL
	DLBase   string // release download base URL
	ExecPath string // binary to replace; defaults to os.Executable()
	OS       string
	Arch     string
	Client   *http.Client
}

func New() *Updater {
	return &Updater{
		Repo:    "schwaggot/sandy",
		APIBase: "https://api.github.com",
		DLBase:  "https://github.com",
		OS:      runtime.GOOS,
		Arch:    runtime.GOARCH,
		Client:  &http.Client{Timeout: 10 * time.Minute},
	}
}

// Run updates the binary to the latest release. It is a no-op for
// development builds and when the binary is already current.
func (u *Updater) Run(current string, out io.Writer) error {
	if current == "dev" {
		_, _ = fmt.Fprintln(out, "skipping binary self-update: development build")
		return nil
	}
	latest, err := u.latestVersion()
	if err != nil {
		return fmt.Errorf("checking latest release: %w", err)
	}
	if !newerThan(latest, current) {
		_, _ = fmt.Fprintf(out, "sandy binary up to date (%s, latest release %s)\n", current, latest)
		return nil
	}

	execPath := u.ExecPath
	if execPath == "" {
		execPath, err = os.Executable()
		if err != nil {
			return fmt.Errorf("locating current executable: %w", err)
		}
		// Replace the real file, not a symlink pointing at it.
		if resolved, rerr := filepath.EvalSymlinks(execPath); rerr == nil {
			execPath = resolved
		}
	}

	_, _ = fmt.Fprintf(out, "updating sandy binary %s -> %s\n", current, latest)

	asset := u.assetName(latest)
	archive, err := u.fetch(u.downloadURL(latest, asset))
	if err != nil {
		return err
	}
	sums, err := u.fetch(u.downloadURL(latest, "checksums.txt"))
	if err != nil {
		return err
	}
	if err := verifyChecksum(archive, sums, asset); err != nil {
		return err
	}
	bin, err := extractBinary(archive, asset, u.OS)
	if err != nil {
		return err
	}
	if err := replaceBinary(execPath, bin, u.OS); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "installed sandy %s to %s\n", latest, execPath)
	return nil
}

func (u *Updater) latestVersion() (string, error) {
	url := fmt.Sprintf("%s/repos/%s/releases/latest", u.APIBase, u.Repo)
	resp, err := u.Client.Get(url)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	var rel struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", err
	}
	if rel.TagName == "" {
		return "", fmt.Errorf("release at %s has no tag_name", url)
	}
	return strings.TrimPrefix(rel.TagName, "v"), nil
}

func (u *Updater) assetName(version string) string {
	ext := "tar.gz"
	if u.OS == "windows" {
		ext = "zip"
	}
	return fmt.Sprintf("sandy_%s_%s_%s.%s", version, u.OS, u.Arch, ext)
}

func (u *Updater) downloadURL(version, file string) string {
	return fmt.Sprintf("%s/%s/releases/download/v%s/%s", u.DLBase, u.Repo, version, file)
}

func (u *Updater) fetch(url string) ([]byte, error) {
	resp, err := u.Client.Get(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// newerThan reports whether latest is strictly newer than current, so a
// source build or snapshot ahead of the newest release is never
// downgraded. Unparseable versions fall back to plain inequality.
func newerThan(latest, current string) bool {
	lv, lok := parseVersion(latest)
	cv, cok := parseVersion(current)
	if !lok || !cok {
		return latest != strings.TrimPrefix(current, "v")
	}
	for i := range lv {
		if lv[i] != cv[i] {
			return lv[i] > cv[i]
		}
	}
	return false
}

// parseVersion parses major.minor.patch, tolerating a leading "v" and
// ignoring any pre-release suffix ("0.3.2-next" parses as 0.3.2).
func parseVersion(v string) ([3]int, bool) {
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexByte(v, '-'); i >= 0 {
		v = v[:i]
	}
	var out [3]int
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

func verifyChecksum(data, sums []byte, name string) error {
	var want string
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == name {
			want = fields[0]
			break
		}
	}
	if want == "" {
		return fmt.Errorf("no checksum entry for %s", name)
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != want {
		return fmt.Errorf("checksum mismatch for %s: want %s, got %s", name, want, got)
	}
	return nil
}

func extractBinary(archive []byte, assetName, goos string) ([]byte, error) {
	binName := "sandy"
	if goos == "windows" {
		binName = "sandy.exe"
	}
	if strings.HasSuffix(assetName, ".zip") {
		zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", assetName, err)
		}
		for _, f := range zr.File {
			if filepath.Base(f.Name) == binName {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				defer func() { _ = rc.Close() }()
				return io.ReadAll(rc)
			}
		}
	} else {
		gz, err := gzip.NewReader(bytes.NewReader(archive))
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", assetName, err)
		}
		defer func() { _ = gz.Close() }()
		tr := tar.NewReader(gz)
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("reading %s: %w", assetName, err)
			}
			if hdr.Typeflag == tar.TypeReg && filepath.Base(hdr.Name) == binName {
				return io.ReadAll(tr)
			}
		}
	}
	return nil, fmt.Errorf("%s not found in %s", binName, assetName)
}

func replaceBinary(path string, data []byte, goos string) error {
	// Write next to the target and rename so the swap is atomic and a
	// running process keeps its old inode.
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".sandy-update-*")
	if err != nil {
		if os.IsPermission(err) {
			return fmt.Errorf("cannot write to %s; rerun with sudo: %w", dir, err)
		}
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o755); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if goos == "windows" {
		// Windows cannot rename over a running executable; move it aside.
		old := path + ".old"
		_ = os.Remove(old)
		if err := os.Rename(path, old); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := os.Rename(tmp.Name(), path); err != nil {
			return err
		}
		_ = os.Remove(old)
		return nil
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		if os.IsPermission(err) {
			return fmt.Errorf("cannot replace %s; rerun with sudo: %w", path, err)
		}
		return err
	}
	return nil
}
