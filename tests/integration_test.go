package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/taskflow-backend/internal/config"
	"github.com/taskflow-backend/internal/db"
	"github.com/taskflow-backend/internal/handler"
	authmw "github.com/taskflow-backend/internal/middleware"
)

var (
	testPool      *pgxpool.Pool
	testJWTSecret = "test-secret-key-for-integration-tests"
	testServer    *httptest.Server
)

func TestMain(m *testing.M) {
	// Use TEST_DATABASE_URL env or skip
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		fmt.Println("TEST_DATABASE_URL not set, skipping integration tests")
		os.Exit(0)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := db.RunMigrations(dbURL, "../migrations"); err != nil {
		fmt.Printf("migrate: %v\n", err)
		os.Exit(1)
	}

	pool, err := db.Connect(ctx, dbURL)
	if err != nil {
		fmt.Printf("connect: %v\n", err)
		os.Exit(1)
	}
	testPool = pool

	r := chi.NewRouter()
	authH := handler.NewAuthHandler(pool, testJWTSecret)
	projectH := handler.NewProjectHandler(pool)
	taskH := handler.NewTaskHandler(pool)

	r.Post("/auth/register", authH.Register)
	r.Post("/auth/login", authH.Login)
	r.Group(func(r chi.Router) {
		r.Use(authmw.Auth(testJWTSecret))
		r.Get("/projects", projectH.List)
		r.Post("/projects", projectH.Create)
		r.Get("/projects/{id}", projectH.Get)
		r.Patch("/projects/{id}", projectH.Update)
		r.Delete("/projects/{id}", projectH.Delete)
		r.Post("/projects/{id}/tasks", taskH.Create)
		r.Get("/projects/{id}/tasks", taskH.List)
		r.Patch("/tasks/{id}", taskH.Update)
		r.Delete("/tasks/{id}", taskH.Delete)
	})

	testServer = httptest.NewServer(r)

	code := m.Run()

	testServer.Close()
	pool.Close()
	os.Exit(code)
}

// ─── Helpers ──────────────────────────────────────────────────────────────

func post(t *testing.T, path string, body interface{}, token string) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, testServer.URL+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	return resp
}

func get(t *testing.T, path, token string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, testServer.URL+path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	return resp
}

func patch(t *testing.T, path string, body interface{}, token string) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPatch, testServer.URL+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH %s: %v", path, err)
	}
	return resp
}

func deleteReq(t *testing.T, path, token string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodDelete, testServer.URL+path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE %s: %v", path, err)
	}
	return resp
}

func decode(t *testing.T, resp *http.Response, v interface{}) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func uniqueEmail() string {
	return fmt.Sprintf("user_%d@test.com", time.Now().UnixNano())
}

// ─── Auth Tests ────────────────────────────────────────────────────────────

func TestRegister_Success(t *testing.T) {
	resp := post(t, "/auth/register", map[string]string{
		"name":     "Alice Test",
		"email":    uniqueEmail(),
		"password": "password123",
	}, "")

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	decode(t, resp, &body)

	if body["token"] == nil {
		t.Error("expected token in response")
	}
	if body["user"] == nil {
		t.Error("expected user in response")
	}
}

func TestRegister_DuplicateEmail(t *testing.T) {
	email := uniqueEmail()
	payload := map[string]string{"name": "Bob", "email": email, "password": "password123"}

	post(t, "/auth/register", payload, "")         // first registration
	resp := post(t, "/auth/register", payload, "") // duplicate

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for duplicate email, got %d", resp.StatusCode)
	}
}

func TestRegister_ValidationErrors(t *testing.T) {
	cases := []struct {
		name    string
		payload map[string]string
	}{
		{"missing name", {"email": "a@b.com", "password": "password123"}},
		{"missing email", {"name": "Alice", "password": "password123"}},
		{"short password", {"name": "Alice", "email": "a@b.com", "password": "short"}},
		{"invalid email", {"name": "Alice", "email": "notanemail", "password": "password123"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := post(t, "/auth/register", tc.payload, "")
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("expected 400 for %s, got %d", tc.name, resp.StatusCode)
			}
		})
	}
}

func TestLogin_Success(t *testing.T) {
	email := uniqueEmail()
	post(t, "/auth/register", map[string]string{
		"name": "Login Test", "email": email, "password": "password123",
	}, "")

	resp := post(t, "/auth/login", map[string]string{
		"email": email, "password": "password123",
	}, "")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	decode(t, resp, &body)
	if body["token"] == nil {
		t.Error("expected token")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	email := uniqueEmail()
	post(t, "/auth/register", map[string]string{
		"name": "BadPass", "email": email, "password": "password123",
	}, "")

	resp := post(t, "/auth/login", map[string]string{
		"email": email, "password": "wrongpassword",
	}, "")

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestProtectedRoute_NoToken(t *testing.T) {
	resp := get(t, "/projects", "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

// ─── Project Tests ─────────────────────────────────────────────────────────

func registerAndLogin(t *testing.T) string {
	t.Helper()
	email := uniqueEmail()
	resp := post(t, "/auth/register", map[string]string{
		"name": "Project Tester", "email": email, "password": "password123",
	}, "")
	var body map[string]interface{}
	decode(t, resp, &body)
	return body["token"].(string)
}

func TestProject_CreateAndGet(t *testing.T) {
	token := registerAndLogin(t)

	// Create
	resp := post(t, "/projects", map[string]string{
		"name": "Test Project", "description": "My test project",
	}, token)

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var proj map[string]interface{}
	decode(t, resp, &proj)
	projectID := proj["id"].(string)

	// Get
	resp2 := get(t, "/projects/"+projectID, token)
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp2.StatusCode)
	}

	var detail map[string]interface{}
	decode(t, resp2, &detail)
	if detail["id"] != projectID {
		t.Errorf("expected project id %s, got %v", projectID, detail["id"])
	}
}

func TestProject_UpdateAndDelete(t *testing.T) {
	token := registerAndLogin(t)

	resp := post(t, "/projects", map[string]string{"name": "To Update"}, token)
	var proj map[string]interface{}
	decode(t, resp, &proj)
	projectID := proj["id"].(string)

	// Update
	patchResp := patch(t, "/projects/"+projectID, map[string]string{"name": "Updated Name"}, token)
	if patchResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on patch, got %d", patchResp.StatusCode)
	}

	var updated map[string]interface{}
	decode(t, patchResp, &updated)
	if updated["name"] != "Updated Name" {
		t.Errorf("expected updated name, got %v", updated["name"])
	}

	// Delete
	delResp := deleteReq(t, "/projects/"+projectID, token)
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", delResp.StatusCode)
	}

	// Should be gone
	getResp := get(t, "/projects/"+projectID, token)
	if getResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", getResp.StatusCode)
	}
}

func TestProject_ForbiddenForNonOwner(t *testing.T) {
	ownerToken := registerAndLogin(t)
	otherToken := registerAndLogin(t)

	resp := post(t, "/projects", map[string]string{"name": "Owner Only"}, ownerToken)
	var proj map[string]interface{}
	decode(t, resp, &proj)
	projectID := proj["id"].(string)

	// Other user tries to delete
	delResp := deleteReq(t, "/projects/"+projectID, otherToken)
	if delResp.StatusCode != http.StatusForbidden && delResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 403 or 404, got %d", delResp.StatusCode)
	}
}

// ─── Task Tests ────────────────────────────────────────────────────────────

func TestTask_CreateListUpdateDelete(t *testing.T) {
	token := registerAndLogin(t)

	// Create project
	resp := post(t, "/projects", map[string]string{"name": "Task Project"}, token)
	var proj map[string]interface{}
	decode(t, resp, &proj)
	projectID := proj["id"].(string)

	// Create task
	taskResp := post(t, "/projects/"+projectID+"/tasks", map[string]interface{}{
		"title":    "My Task",
		"priority": "high",
	}, token)

	if taskResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", taskResp.StatusCode)
	}

	var task map[string]interface{}
	decode(t, taskResp, &task)
	taskID := task["id"].(string)

	if task["status"] != "todo" {
		t.Errorf("expected default status todo, got %v", task["status"])
	}

	// List tasks
	listResp := get(t, "/projects/"+projectID+"/tasks", token)
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", listResp.StatusCode)
	}

	// Update task status
	updateResp := patch(t, "/tasks/"+taskID, map[string]string{"status": "in_progress"}, token)
	if updateResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on task update, got %d", updateResp.StatusCode)
	}

	var updatedTask map[string]interface{}
	decode(t, updateResp, &updatedTask)
	if updatedTask["status"] != "in_progress" {
		t.Errorf("expected in_progress, got %v", updatedTask["status"])
	}

	// Delete task
	delResp := deleteReq(t, "/tasks/"+taskID, token)
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", delResp.StatusCode)
	}
}

func TestTask_InvalidStatus(t *testing.T) {
	token := registerAndLogin(t)

	resp := post(t, "/projects", map[string]string{"name": "Validate Project"}, token)
	var proj map[string]interface{}
	decode(t, resp, &proj)
	projectID := proj["id"].(string)

	taskResp := post(t, "/projects/"+projectID+"/tasks", map[string]interface{}{
		"title":    "Bad Task",
		"priority": "invalid_priority",
	}, token)

	if taskResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid priority, got %d", taskResp.StatusCode)
	}
}

func TestTask_FilterByStatus(t *testing.T) {
	token := registerAndLogin(t)

	resp := post(t, "/projects", map[string]string{"name": "Filter Project"}, token)
	var proj map[string]interface{}
	decode(t, resp, &proj)
	projectID := proj["id"].(string)

	// Create two tasks
	post(t, "/projects/"+projectID+"/tasks", map[string]interface{}{
		"title": "Todo Task", "priority": "low",
	}, token)

	tr := post(t, "/projects/"+projectID+"/tasks", map[string]interface{}{
		"title": "Done Task", "priority": "medium",
	}, token)
	var doneTask map[string]interface{}
	decode(t, tr, &doneTask)
	patch(t, "/tasks/"+doneTask["id"].(string), map[string]string{"status": "done"}, token)

	// Filter by todo
	listResp := get(t, "/projects/"+projectID+"/tasks?status=todo", token)
	var result map[string]interface{}
	decode(t, listResp, &result)

	data, _ := result["data"].([]interface{})
	for _, item := range data {
		task := item.(map[string]interface{})
		if task["status"] != "todo" {
			t.Errorf("filter returned non-todo task: %v", task["status"])
		}
	}
}

// ─── Config test helper ────────────────────────────────────────────────────

var _ = config.Load // ensure config package is linked
