package gitops_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zealllot/hac/internal/gitops"
)

// initRepo creates a git repo in a temp dir with an initial committed file
// and returns the repo path.
func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"git", "init", "-q"},
		{"git", "config", "user.email", "test@example.com"},
		{"git", "config", "user.name", "Test"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	// Seed with an initial commit so HEAD exists.
	seed := filepath.Join(dir, "seed.txt")
	if err := os.WriteFile(seed, []byte("seed\n"), 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	for _, args := range [][]string{
		{"git", "add", "seed.txt"},
		{"git", "commit", "-q", "-m", "init"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func TestCommit_createsCommit(t *testing.T) {
	repo := initRepo(t)
	target := filepath.Join(repo, "automations", "light.yaml")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(target, []byte("alias: x\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := gitops.Add(target); err != nil {
		t.Fatalf("Add: %v", err)
	}

	if err := gitops.Commit(repo, "feat: 客厅 光亮 关灯"); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Last commit subject equals the message we passed.
	cmd := exec.Command("git", "log", "-1", "--pretty=%s")
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v\n%s", err, out)
	}
	got := strings.TrimSpace(string(out))
	if got != "feat: 客厅 光亮 关灯" {
		t.Errorf("commit message = %q, want %q", got, "feat: 客厅 光亮 关灯")
	}
	// Working tree should be clean now.
	cmd = exec.Command("git", "status", "--porcelain")
	cmd.Dir = repo
	out, _ = cmd.CombinedOutput()
	if strings.TrimSpace(string(out)) != "" {
		t.Errorf("working tree not clean after Commit:\n%s", out)
	}
}

func TestAdd_outsideRepoErrors(t *testing.T) {
	dir := t.TempDir() // no git init
	target := filepath.Join(dir, "x.yaml")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	err := gitops.Add(target)
	if err == nil {
		t.Fatalf("expected error for file outside any git repo")
	}
	if !strings.Contains(err.Error(), "git add") {
		t.Errorf("err = %q, want substring 'git add'", err.Error())
	}
}

func TestAdd_stagesFile(t *testing.T) {
	repo := initRepo(t)
	target := filepath.Join(repo, "automations", "light.yaml")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(target, []byte("alias: x\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := gitops.Add(target); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Verify staged via `git status --porcelain` — should show "A " (added) for our file.
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git status: %v\n%s", err, out)
	}
	got := string(out)
	if !strings.Contains(got, "A  automations/light.yaml") {
		t.Errorf("file not staged; git status output:\n%s", got)
	}
}
