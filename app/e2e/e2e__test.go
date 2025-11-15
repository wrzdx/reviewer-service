package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"testing"

	"reviewer-service/app/db"
	"reviewer-service/app/testutils"
)

var baseURL string

func TestMain(m *testing.M) {
	if err := testutils.LoadTestEnv("../../.env"); err != nil {
		log.Fatalf("failed to load env: %v", err)
	}

	if err := db.Init(); err != nil {
		panic(err)
	}
	if err := db.ClearAllTables(); err != nil {
		log.Fatalf("Failed to clear database: %v", err)
	}
	appHost := os.Getenv("APP_HOST")
	if appHost == "" {
		appHost = "localhost"
	}
	baseURL = fmt.Sprintf("http://%s:8080", appHost)

	db.Close()
	os.Exit(m.Run())
}

func postJSON(t *testing.T, path string, payload any) *http.Response {
	ctx := context.Background()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("failed to create POST request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST %s failed: %v", path, err)
	}
	return resp
}

func getJSON(t *testing.T, path string) *http.Response {
	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, http.NoBody)
	if err != nil {
		t.Fatalf("failed to create GET request: %v", err)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET %s failed: %v", path, err)
	}
	return resp
}

func TestE2E_CreateTeamUserAndPR(t *testing.T) {
	teamPayload := map[string]any{
		"team_name": "backend",
		"members": []map[string]any{
			{"user_id": "u1", "username": "Alice", "is_active": true},
			{"user_id": "u2", "username": "Bob", "is_active": true},
		},
	}
	resp := postJSON(t, "/team/add", teamPayload)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	prPayload := map[string]any{
		"pull_request_id":   "pr-1001",
		"pull_request_name": "Add search",
		"author_id":         "u1",
	}
	resp = postJSON(t, "/pullRequest/create", prPayload)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	resp = getJSON(t, "/users/getReview?user_id=u2")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result struct {
		UserID       string `json:"user_id"`
		PullRequests []struct {
			PullRequestID   string `json:"pull_request_id"`
			PullRequestName string `json:"pull_request_name"`
			Status          string `json:"status"`
		} `json:"pull_requests"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(result.PullRequests) == 0 || result.PullRequests[0].PullRequestID != "pr-1001" {
		t.Fatalf("expected u2 to have PR pr-1001 assigned")
	}

	pr := result.PullRequests[0]
	if pr.Status != "OPEN" {
		t.Fatalf("expected PR status OPEN, got %s", pr.Status)
	}
}

func TestE2E_ReassignReviewer(t *testing.T) {
	teamPayload := map[string]any{
		"team_name": "frontend",
		"members": []map[string]any{
			{"user_id": "u3", "username": "Charlie", "is_active": true},
			{"user_id": "u4", "username": "Dana", "is_active": true},
		},
	}
	resp := postJSON(t, "/team/add", teamPayload)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	prPayload := map[string]any{
		"pull_request_id":   "pr-2001",
		"pull_request_name": "Refactor UI",
		"author_id":         "u3",
	}
	resp = postJSON(t, "/pullRequest/create", prPayload)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	reassignPayload := map[string]any{
		"pull_request_id": "pr-2001",
		"old_user_id":     "u4",
	}
	resp = postJSON(t, "/pullRequest/reassign", reassignPayload)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 408, got %d", resp.StatusCode)
	}

	resp = getJSON(t, "/users/getReview?user_id=u4")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result struct {
		UserID       string `json:"user_id"`
		PullRequests []struct {
			PullRequestID string `json:"pull_request_id"`
		} `json:"pull_requests"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	defer resp.Body.Close()

	found := false
	for _, pr := range result.PullRequests {
		if pr.PullRequestID == "pr-2001" {
			found = true
			break
		}
	}

	if !found {
		t.Fatalf("expected PR pr-2001 to be assigned to u4")
	}
}
