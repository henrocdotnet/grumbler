package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/henrocdotnet/grumbler/internal/model"
)

const testDiff = `diff --git a/main.go b/main.go
index abc1234..def5678 100644
--- a/main.go
+++ b/main.go
@@ -1,5 +1,6 @@
 package main

+import "fmt"
+
 func main() {
-    println("hello")
+    fmt.Println("hello")
 }
diff --git a/util.go b/util.go
new file mode 100644
--- /dev/null
+++ b/util.go
@@ -0,0 +1,3 @@
+package main
+
+func helper() {}
`

func TestParseDiff(t *testing.T) {
	files := ParseDiff(testDiff)
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}

	if files[0].Path != "main.go" {
		t.Errorf("file 0 path: got %q, want %q", files[0].Path, "main.go")
	}
	if files[0].Status != model.FileModified {
		t.Errorf("file 0 status: got %q, want %q", files[0].Status, model.FileModified)
	}
	if files[0].Language != "go" {
		t.Errorf("file 0 language: got %q, want %q", files[0].Language, "go")
	}

	if files[1].Path != "util.go" {
		t.Errorf("file 1 path: got %q, want %q", files[1].Path, "util.go")
	}
	if files[1].Status != model.FileAdded {
		t.Errorf("file 1 status: got %q, want %q", files[1].Status, model.FileAdded)
	}
}

func TestAnnotatePatch(t *testing.T) {
	patch := `@@ -1,3 +1,4 @@
 line1
+added
 line2
 line3
`
	result := AnnotatePatch(patch)
	if result == "" {
		t.Fatal("empty result")
	}
	// Should contain line numbers
	if !containsSubstring(result, "L1") {
		t.Error("missing line number annotation")
	}
}

func TestResolveRemoteRefSlashedBranch(t *testing.T) {
	dir := initGitRepo(t)
	runGit(t, dir, "remote", "add", "origin", "https://example.invalid/repo.git")
	runGit(t, dir, "update-ref", "refs/remotes/origin/release/2026-05-07", "HEAD")

	got := resolveRemoteRef(dir, "release/2026-05-07")
	if got != "origin/release/2026-05-07" {
		t.Fatalf("resolveRemoteRef: got %q, want origin/release/2026-05-07", got)
	}
}

func TestResolveRemoteRefQualifiedRemote(t *testing.T) {
	dir := initGitRepo(t)
	runGit(t, dir, "remote", "add", "origin", "https://example.invalid/repo.git")

	got := resolveRemoteRef(dir, "origin/release/2026-05-07")
	if got != "origin/release/2026-05-07" {
		t.Fatalf("resolveRemoteRef: got %q, want origin/release/2026-05-07", got)
	}
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@example.invalid")
	runGit(t, dir, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("test\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "initial")
	return dir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && searchSubstring(s, sub)
}

func searchSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
