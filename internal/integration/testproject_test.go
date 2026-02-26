//go:build integration

package integration_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/henrocdotnet/grumbler/internal/git"
)

const testProjectConfig = `llm:
  provider: claude-cli
  model: claude-sonnet-4-6

review:
  baseBranch: main
  mode: diff

passes:
  expertPanel: false
  vet: false
  crossFile: false

filter:
  minSeverity: low

output:
  format: terminal
`

const buggyServiceCode = `package main

import (
	"database/sql"
	"fmt"
	"net/http"
)

// UserService handles user data access.
type UserService struct {
	db *sql.DB
}

// GetUser retrieves a user by ID.
func (s *UserService) GetUser(id string) string {
	var name string
	// SQL injection: query built by string concatenation
	row := s.db.QueryRow("SELECT name FROM users WHERE id = " + id)
	row.Scan(&name)
	return name
}

// Divide returns a divided by b.
func Divide(a, b int) int {
	// no zero-divisor check
	return a / b
}

// HandleRequest serves user lookups over HTTP.
func HandleRequest(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	svc := &UserService{}
	name := svc.GetUser(id)
	fmt.Fprintf(w, "Hello, %s", name)
}

func main() {
	http.HandleFunc("/user", HandleRequest)
	http.ListenAndServe(":8080", nil)
}
`

// testProjectDir returns the path to the synthetic test project,
// creating and initialising it if it does not already exist.
func testProjectDir(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	root, err := git.TopLevel(wd)
	if err != nil {
		t.Fatalf("git top-level: %v", err)
	}

	projDir := filepath.Join(root, ".test-project")

	if _, err := os.Stat(filepath.Join(projDir, ".git")); err == nil {
		return projDir // already initialised
	}

	initTestProject(t, projDir)
	return projDir
}

func initTestProject(t *testing.T, dir string) {
	t.Helper()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("cmd %v: %v\n%s", args, err, out)
		}
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}

	run("git", "init", "-b", "main")
	run("git", "config", "user.email", "test@grumbler.local")
	run("git", "config", "user.name", "Grumbler Test")

	grumblerDir := filepath.Join(dir, ".grumbler")
	if err := os.MkdirAll(grumblerDir, 0o755); err != nil {
		t.Fatalf("mkdir .grumbler: %v", err)
	}
	writeTestFile(t, filepath.Join(grumblerDir, "config.yaml"), testProjectConfig)

	run("git", "add", ".")
	run("git", "commit", "--allow-empty", "-m", "Initial commit")

	run("git", "checkout", "-b", "feature/buggy-service")
	writeTestFile(t, filepath.Join(dir, "service.go"), buggyServiceCode)
	run("git", "add", "service.go")
	run("git", "commit", "-m", "Add user service")
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
