// Package gitops is a thin wrapper around `git add` and `git commit`,
// scoped to the file's repository. Used by `hac deploy` to keep the local
// config repo in sync with HA after a successful push.
package gitops

import (
	"fmt"
	"os/exec"
	"path/filepath"
)

// Add stages the given file via `git add`. The git invocation runs from the
// file's directory so git auto-discovers the enclosing repository.
func Add(file string) error {
	dir := filepath.Dir(file)
	cmd := exec.Command("git", "add", filepath.Base(file))
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add %s: %w: %s", file, err, string(out))
	}
	return nil
}

// Commit creates a commit in the repository containing dir, with the given
// message. Use after Add.
func Commit(dir, message string) error {
	cmd := exec.Command("git", "commit", "-m", message)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit: %w: %s", err, string(out))
	}
	return nil
}
