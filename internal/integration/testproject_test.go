//go:build integration

package integration_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/henrocdotnet/grumbler/internal/config"
	"github.com/henrocdotnet/grumbler/internal/git"
)

const buggyAuthCode = `package main

import (
	"crypto/md5"
	"fmt"
)

const adminPassword = "super_secret_123"

// User holds account credentials in plaintext.
type User struct {
	Username string
	Password string
	Role     string
}

// Authenticate checks credentials against a hardcoded admin password.
func Authenticate(username, password string) bool {
	if username == "admin" && password == adminPassword {
		return true
	}
	return false
}

// HashPassword uses MD5, a broken algorithm unsuitable for passwords.
func HashPassword(password string) string {
	h := md5.New()
	h.Write([]byte(password))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// IsAdmin performs a case-sensitive role check; "Admin" would fail.
func IsAdmin(u User) bool {
	return u.Role == "admin"
}
`

const buggyCacheCode = `package cache

import "time"

// Cache is an in-memory store with no concurrency protection.
type Cache struct {
	data map[string]cacheEntry // no mutex — data race under concurrent access
}

type cacheEntry struct {
	value   string
	expires time.Time
}

// Set stores a value; the cache grows without bound and is never evicted.
func (c *Cache) Set(key, value string, ttl time.Duration) {
	c.data[key] = cacheEntry{value: value, expires: time.Now().Add(ttl)}
}

// Get returns a value without checking whether the entry has expired.
func (c *Cache) Get(key string) (string, bool) {
	e, ok := c.data[key]
	return e.value, ok
}

// MustGet returns a value or panics — inappropriate panic in library code.
func (c *Cache) MustGet(key string) string {
	e, ok := c.data[key]
	if !ok {
		panic("cache: key not found: " + key)
	}
	return e.value
}

// NewCache returns a Cache whose map is nil; the first Set call will panic.
func NewCache() *Cache {
	return &Cache{}
}
`

const buggyAPICode = `package main

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
)

// FetchUser calls an external service without a timeout or input validation.
func FetchUser(id string) (map[string]interface{}, error) {
	resp, err := http.Get("https://api.example.com/users/" + id)
	if err != nil {
		return nil, err
	}
	// response body is never closed
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	json.Unmarshal(body, &result) // unmarshal error silently discarded
	return result, nil
}

// WriteResponse serializes data and writes it, ignoring all errors.
func WriteResponse(w http.ResponseWriter, data interface{}) {
	bytes, _ := json.Marshal(data)
	w.Write(bytes)
}
`

const buggyLoggerCode = `package main

import (
	"fmt"
	"os"
)

var logFile *os.File // global; never closed

// InitLogger opens the log file, silently discarding any open error.
func InitLogger(path string) {
	logFile, _ = os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
}

// Log writes to the global log file; panics if InitLogger was not called.
func Log(msg string) {
	fmt.Fprintln(logFile, msg)
}

// LogSensitive records user credentials in plaintext.
func LogSensitive(user, password string) {
	Log(fmt.Sprintf("login attempt: user=%s password=%s", user, password))
}
`

// mediumHandlersCode exercises MEDIUM-severity patterns:
// non-atomic counter under concurrency, magic numbers, discarded encode error.
const mediumHandlersCode = `package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// server holds HTTP handler dependencies.
type server struct {
	startedAt time.Time
	requests  int64 // incremented without sync/atomic — data race under load
}

// HandleList returns a hardcoded paginated result set.
func (s *server) HandleList(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	s.requests++ // non-atomic: lost updates under concurrent requests

	offset := (page - 1) * 100 // magic number: page size not configurable
	results := make([]string, 0, 100)
	for i := offset; i < offset+100; i++ {
		results = append(results, fmt.Sprintf("item-%d", i))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results) // encode error silently discarded
}

// HandleHealth writes plain-text uptime; no structured response.
func (s *server) HandleHealth(w http.ResponseWriter, r *http.Request) {
	uptime := time.Since(s.startedAt)
	fmt.Fprintf(w, "ok uptime=%s requests=%d", uptime, s.requests)
}

// Retry calls fn up to 3 times with a 500 ms fixed delay.
func Retry(fn func() error) error {
	for i := 0; i < 3; i++ { // magic number: retry count and delay not configurable
		if err := fn(); err == nil {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("all retries exhausted")
}

// FetchData fetches the given URL. A new http.Client is created per call,
// bypassing connection pooling and forcing a fresh TCP handshake every request.
func FetchData(url string) ([]byte, error) {
	client := &http.Client{Timeout: 5 * time.Second} // new client per call: no connection reuse
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}
`

// lowConfigCode exercises LOW-severity patterns:
// missing godoc, always-overwrite zero-value merge, duplicated magic number.
const lowConfigCode = `package config

import (
	"os"
	"strconv"
	"strings"
)

// AppConfig holds application settings.
type AppConfig struct {
	Host  string
	Port  int
	Debug bool
}

func load() *AppConfig {
	portStr := os.Getenv("APP_PORT")
	port, _ := strconv.Atoi(portStr) // parse error silently discarded
	if port == 0 {
		port = 8080 // magic number duplicated below
	}
	return &AppConfig{
		Host:  os.Getenv("APP_HOST"),
		Port:  port,
		Debug: strings.ToLower(os.Getenv("APP_DEBUG")) == "true",
	}
}

// Defaults returns a baseline AppConfig.
func Defaults() *AppConfig {
	return &AppConfig{
		Host:  "localhost",
		Port:  8080, // duplicated magic number; no shared constant
		Debug: false,
	}
}

// Merge overlays non-zero fields of src onto dst.
func Merge(dst, src *AppConfig) {
	if src.Host != "" {
		dst.Host = src.Host
	}
	dst.Port = src.Port   // always overwrites; zero value in src clears dst.Port
	dst.Debug = src.Debug // always overwrites; cannot distinguish unset from false
}
`

// goodStringsCode is clean, idiomatic Go — should produce no review issues.
const goodStringsCode = `// Package utils provides general-purpose helper functions.
package utils

import (
	"strings"
	"unicode"
)

// Truncate shortens s to at most n runes, appending an ellipsis if truncated.
func Truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}

// ToSnakeCase converts a camelCase or PascalCase identifier to snake_case.
func ToSnakeCase(s string) string {
	var b strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) && i > 0 {
			b.WriteByte('_')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// ContainsAny reports whether s contains any of the given substrings.
func ContainsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
`

// goodMathCode is clean, idiomatic generic Go — should produce no review issues.
const goodMathCode = `package utils

// Clamp returns v constrained to [lo, hi].
func Clamp[T int | float64](v, lo, hi T) T {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Sum returns the sum of all provided values.
func Sum[T int | float64](vals ...T) T {
	var total T
	for _, v := range vals {
		total += v
	}
	return total
}

// Map applies fn to each element and returns a new slice of results.
func Map[T, U any](slice []T, fn func(T) U) []U {
	out := make([]U, len(slice))
	for i, v := range slice {
		out[i] = fn(v)
	}
	return out
}
`

// performanceStoreCode exercises PERFORMANCE-severity patterns:
// N+1 queries, unbounded collection growth, O(n) primality with no memoization.
const performanceStoreCode = `package store

import "database/sql"

// GetOrdersWithItems loads all orders then fetches items per order — N+1 queries.
func GetOrdersWithItems(db *sql.DB) ([]Order, error) {
	rows, err := db.Query("SELECT id, user_id FROM orders")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []Order
	for rows.Next() {
		var o Order
		if err := rows.Scan(&o.ID, &o.UserID); err != nil {
			return nil, err
		}
		// N+1: one round-trip per order instead of a JOIN or IN clause
		itemRows, err := db.Query("SELECT id, name FROM items WHERE order_id = ?", o.ID)
		if err != nil {
			return nil, err
		}
		for itemRows.Next() {
			var item Item
			itemRows.Scan(&item.ID, &item.Name)
			o.Items = append(o.Items, item)
		}
		itemRows.Close()
		orders = append(orders, o)
	}
	return orders, nil
}

// AllUserIDs loads every user ID into memory with no limit or pagination.
func AllUserIDs(db *sql.DB) ([]int64, error) {
	rows, err := db.Query("SELECT id FROM users") // unbounded: millions of rows possible
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		rows.Scan(&id)
		ids = append(ids, id)
	}
	return ids, nil
}

// IsPrime uses trial division with no memoization or early sqrt bound.
func IsPrime(n int) bool {
	if n < 2 {
		return false
	}
	for i := 2; i < n; i++ { // O(n) per call; no cache
		if n%i == 0 {
			return false
		}
	}
	return true
}

// TaggedProducts fetches all products and calls IsPrime per row — redundant
// repeated computation with no result cache.
func TaggedProducts(db *sql.DB) ([]Product, error) {
	rows, err := db.Query("SELECT id, name, price FROM products")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var products []Product
	idx := 0
	for rows.Next() {
		var p Product
		rows.Scan(&p.ID, &p.Name, &p.Price)
		if IsPrime(idx) { // recomputed every call, no memoization
			p.Name = "[featured] " + p.Name
		}
		products = append(products, p)
		idx++
	}
	return products, nil
}

type Order struct {
	ID     int64
	UserID int64
	Items  []Item
}

type Item struct {
	ID   int64
	Name string
}

type Product struct {
	ID    int64
	Name  string
	Price float64
}
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
	// TODO: remove this debug logging before release
	fmt.Println("DEBUG: handling request for id:", id)
	svc := &UserService{}
	name := svc.GetUser(id)
	fmt.Fprintf(w, "Hello, %s", name)
}

func main() {
	http.HandleFunc("/user", HandleRequest)
	http.ListenAndServe(":8080", nil)
}
`

const buggyDeployCode = `package main

import (
	"fmt"
	"os/exec"
)

// Deploy shells out to a deploy script, concatenating user input into the command.
func Deploy(service string) error {
	cmd := exec.Command("bash", "-c", "deploy.sh --service "+service)
	return cmd.Run()
}

// Ping shells out to ping, concatenating a user-supplied host into the command.
func Ping(host string) (string, error) {
	out, err := exec.Command("sh", "-c", "ping -c1 "+host).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ping %s: %w", host, err)
	}
	return string(out), nil
}
`

const buggyDecodeCode = `package main

import (
	"encoding/gob"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
)

// LoadSession deserializes a gob-encoded session from an untrusted file
// into an interface{} — insecure deserialization.
func LoadSession(path string) (interface{}, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var session interface{}
	if err := gob.NewDecoder(f).Decode(&session); err != nil {
		return nil, err
	}
	return session, nil
}

// DecodeBody reads the full request body with no size limit and unmarshals
// into an untyped interface{} — no schema validation.
func DecodeBody(r *http.Request) (interface{}, error) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	var payload interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}
`

const buggyServiceTestCode = `package main

import "testing"

// TestHandleRequest_HappyPath only tests the success path;
// no test for missing ID, invalid ID, or nil db.
func TestHandleRequest_HappyPath(t *testing.T) {
	// In a real test we would spin up an httptest.Server;
	// here we just verify the function signature compiles.
	_ = HandleRequest
}

// TestDivide only tests the happy path; no test for b==0.
func TestDivide(t *testing.T) {
	got := Divide(10, 2)
	if got != 5 {
		t.Errorf("Divide(10,2) = %d, want 5", got)
	}
}
`

const stableAPITypesCode = `package main

// UserResponse is the public API response for a user.
type UserResponse struct {
	ID    int64  ` + "`json:\"id\"`" + `
	Name  string ` + "`json:\"name\"`" + `
	Email string ` + "`json:\"email\"`" + `
}

// ListUsers returns up to limit users.
func ListUsers(limit int) ([]UserResponse, error) {
	return nil, nil
}

// GetUserByID returns a single user by numeric ID.
func GetUserByID(id int64) (*UserResponse, error) {
	return nil, nil
}
`

const breakingAPITypesCode = `package main

// UserResponse is the public API response for a user.
// BREAKING: Name renamed to FullName, Email field removed.
type UserResponse struct {
	ID       int64  ` + "`json:\"id\"`" + `
	FullName string ` + "`json:\"full_name\"`" + `
}

// ListOpts replaces the simple limit parameter.
type ListOpts struct {
	Limit  int
	Offset int
}

// ListUsers now takes ListOpts instead of int — breaking signature change.
func ListUsers(opts ListOpts) ([]UserResponse, error) {
	return nil, nil
}

// GetUserByID now takes string instead of int64 — breaking signature change.
func GetUserByID(id string) (*UserResponse, error) {
	return nil, nil
}
`

const fixtureVersion = "3"

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

	// Fixture version guard: reinitialise if absent or stale.
	versionFile := filepath.Join(projDir, ".fixture-version")
	if v, err := os.ReadFile(versionFile); err == nil && string(v) == fixtureVersion {
		return projDir // up-to-date
	}
	// Stale or missing — wipe and recreate.
	os.RemoveAll(projDir)

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

	if err := config.InitProject(filepath.Join(dir, config.Dir)); err != nil {
		t.Fatalf("config.InitProject: %v", err)
	}

	// API baseline on main (for API-001 breaking-change detection).
	writeTestFile(t, filepath.Join(dir, "api_types.go"), stableAPITypesCode)

	run("git", "add", ".")
	run("git", "commit", "--allow-empty", "-m", "Initial commit")

	run("git", "checkout", "-b", "feature/buggy-service")

	// Root-level files — CRITICAL and HIGH severity issues.
	writeTestFile(t, filepath.Join(dir, "service.go"), buggyServiceCode)
	writeTestFile(t, filepath.Join(dir, "auth.go"), buggyAuthCode)
	writeTestFile(t, filepath.Join(dir, "cache.go"), buggyCacheCode)
	writeTestFile(t, filepath.Join(dir, "api.go"), buggyAPICode)
	writeTestFile(t, filepath.Join(dir, "logger.go"), buggyLoggerCode)

	// Nested: handlers/ — MEDIUM severity issues.
	if err := os.MkdirAll(filepath.Join(dir, "handlers"), 0o755); err != nil {
		t.Fatalf("mkdir handlers: %v", err)
	}
	writeTestFile(t, filepath.Join(dir, "handlers", "http.go"), mediumHandlersCode)

	// Nested: store/ — PERFORMANCE severity issues.
	if err := os.MkdirAll(filepath.Join(dir, "store"), 0o755); err != nil {
		t.Fatalf("mkdir store: %v", err)
	}
	writeTestFile(t, filepath.Join(dir, "store", "queries.go"), performanceStoreCode)

	// Nested: internal/config/ — LOW severity issues.
	if err := os.MkdirAll(filepath.Join(dir, "internal", "config"), 0o755); err != nil {
		t.Fatalf("mkdir internal/config: %v", err)
	}
	writeTestFile(t, filepath.Join(dir, "internal", "config", "loader.go"), lowConfigCode)

	// Nested: pkg/utils/ — clean code, should produce no issues.
	if err := os.MkdirAll(filepath.Join(dir, "pkg", "utils"), 0o755); err != nil {
		t.Fatalf("mkdir pkg/utils: %v", err)
	}
	writeTestFile(t, filepath.Join(dir, "pkg", "utils", "strings.go"), goodStringsCode)
	writeTestFile(t, filepath.Join(dir, "pkg", "utils", "math.go"), goodMathCode)

	// New files for SEC-003, SEC-004, TEST-001, API-001.
	writeTestFile(t, filepath.Join(dir, "deploy.go"), buggyDeployCode)
	writeTestFile(t, filepath.Join(dir, "decode.go"), buggyDecodeCode)
	writeTestFile(t, filepath.Join(dir, "service_test.go"), buggyServiceTestCode)
	writeTestFile(t, filepath.Join(dir, "api_types.go"), breakingAPITypesCode)

	run("git", "add",
		"service.go", "auth.go", "cache.go", "api.go", "logger.go",
		"handlers/http.go",
		"store/queries.go",
		"internal/config/loader.go",
		"pkg/utils/strings.go", "pkg/utils/math.go",
		"deploy.go", "decode.go", "service_test.go", "api_types.go",
	)
	run("git", "commit", "-m", "Add user service, auth, cache, API client, and logger")

	// Write fixture version marker (not tracked by git).
	writeTestFile(t, filepath.Join(dir, ".fixture-version"), fixtureVersion)
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
