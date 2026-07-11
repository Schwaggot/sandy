package project

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// Root walks up from cwd until it finds .sandy/ or .git/, otherwise returns cwd.
// The walk stops before reaching the user's home directory: ~/.sandy is sandy's
// global config dir and a dotfiles repo may put .git at $HOME, so neither may
// mark $HOME as a project root (which would mount the whole home directory).
func Root(cwd string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	return rootFrom(cwd, home)
}

func rootFrom(cwd, home string) string {
	dir := filepath.Clean(cwd)
	if home != "" {
		home = filepath.Clean(home)
	}
	for {
		if dir == home {
			return cwd
		}
		if _, err := os.Stat(filepath.Join(dir, ".sandy")); err == nil {
			return dir
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return cwd
		}
		dir = parent
	}
}

// Hash returns a stable short identifier for the project path.
func Hash(root string) string {
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	sum := sha256.Sum256([]byte(abs))
	return hex.EncodeToString(sum[:])[:16]
}

// DetectToolchains inspects common project files.
// Returns a deduped slice of "python", "cpp", "node".
func DetectToolchains(root string) []string {
	checks := map[string][]string{
		"python": {"pyproject.toml", "requirements.txt", "setup.py", "Pipfile"},
		"cpp":    {"CMakeLists.txt", "conanfile.txt", "conanfile.py"},
		"node":   {"package.json"},
	}
	var found []string
	for tc, files := range checks {
		for _, f := range files {
			if _, err := os.Stat(filepath.Join(root, f)); err == nil {
				found = append(found, tc)
				break
			}
		}
	}
	return found
}

// PickToolchain returns the best single toolchain tag for a set of detections.
func PickToolchain(detected []string) string {
	if len(detected) == 0 {
		return "fullstack"
	}
	if len(detected) == 1 {
		return detected[0]
	}
	return "fullstack"
}

func Cwd() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("cannot determine working directory: %w", err)
	}
	return cwd, nil
}
