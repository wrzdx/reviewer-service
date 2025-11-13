package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"reviewer-service/app/db"
	"reviewer-service/app/models"
	"time"
)

func CreatePRHandler(w http.ResponseWriter, r *http.Request) {
	var req models.CreatePRRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	ctx := context.Background()

	var teamName string
	err := db.Pool.QueryRow(ctx, "SELECT team_name FROM users WHERE user_id=$1", req.AuthorID).Scan(&teamName)
	if err != nil {
		http.Error(w, `{"error":{"code":"NOT_FOUND","message":"author or team not found"}}`, http.StatusNotFound)
		return
	}

	rows, err := db.Pool.Query(ctx, `
		SELECT user_id FROM users
		WHERE team_name=$1 AND is_active=true AND user_id<>$2
	`, teamName, req.AuthorID)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	candidates := []string{}
	for rows.Next() {
		var u string
		if err = rows.Scan(&u); err == nil {
			candidates = append(candidates, u)
		}
	}

	assigned := []string{}
	for i := 0; i < 2 && len(candidates) > 0; i++ {
		assigned = append(assigned, candidates[0])
		candidates = candidates[1:]
	}

	_, err = db.Pool.Exec(ctx, `
		INSERT INTO pull_requests(pull_request_id, pull_request_name, author_id, status, assigned_reviewers)
		VALUES($1,$2,$3,'OPEN',$4)
	`, req.PullRequestID, req.PullRequestName, req.AuthorID, assigned)
	if err != nil {
		http.Error(w, `{"error":{"code":"PR_EXISTS","message":"PR id already exists"}}`, http.StatusConflict)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	err = json.NewEncoder(w).Encode(map[string]any{
		"pr": models.PullRequest{
			PullRequestID:     req.PullRequestID,
			PullRequestName:   req.PullRequestName,
			AuthorID:          req.AuthorID,
			Status:            "OPEN",
			AssignedReviewers: assigned,
		},
	})
	if err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
}

func MergePRHandler(w http.ResponseWriter, r *http.Request) {
	var req models.MergePRRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	ctx := context.Background()

	var status string
	err := db.Pool.QueryRow(ctx, "SELECT status FROM pull_requests WHERE pull_request_id=$1", req.PullRequestID).Scan(&status)
	if err != nil {
		http.Error(w, `{"error":{"code":"NOT_FOUND","message":"PR not found"}}`, http.StatusNotFound)
		return
	}

	if status != "MERGED" {
		_, _ = db.Pool.Exec(ctx, `
			UPDATE pull_requests
			SET status='MERGED', merged_at=NOW()
			WHERE pull_request_id=$1
		`, req.PullRequestID)
	}

	var pr models.PullRequest
	var reviewers []string
	var mergedAt *time.Time

	err = db.Pool.QueryRow(ctx, `
		SELECT pull_request_id, pull_request_name, author_id, status, assigned_reviewers, merged_at
		FROM pull_requests
		WHERE pull_request_id=$1
	`, req.PullRequestID).Scan(
		&pr.PullRequestID,
		&pr.PullRequestName,
		&pr.AuthorID,
		&pr.Status,
		&reviewers,
		&mergedAt,
	)

	if err != nil {
		log.Printf("MergePRHandler: failed to fetch PR %s: %v", req.PullRequestID, err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	pr.AssignedReviewers = reviewers
	if mergedAt != nil {
		pr.MergedAt = mergedAt
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"pr": pr}); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
}

func ReassignPRHandler(w http.ResponseWriter, r *http.Request) {
	var req models.ReassignPRRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	ctx := context.Background()

	var status string
	var assigned []string
	err := db.Pool.QueryRow(ctx, `
		SELECT status, assigned_reviewers FROM pull_requests WHERE pull_request_id=$1
	`, req.PullRequestID).Scan(&status, &assigned)
	if err != nil {
		http.Error(w, `{"error":{"code":"NOT_FOUND","message":"PR not found"}}`, http.StatusNotFound)
		return
	}

	if status == "MERGED" {
		http.Error(w, `{"error":{"code":"PR_MERGED","message":"cannot reassign on merged PR"}}`, http.StatusConflict)
		return
	}

	found := false
	for i, uid := range assigned {
		if uid != req.OldReviewerID {
			continue
		}
		found = true

		var teamName string
		err = db.Pool.QueryRow(ctx, "SELECT team_name FROM users WHERE user_id=$1", req.OldReviewerID).Scan(&teamName)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		rows, _ := db.Pool.Query(ctx, `
			SELECT user_id FROM users WHERE team_name=$1 AND is_active=true AND user_id<>$2
		`, teamName, req.OldReviewerID)
		candidates := []string{}
		for rows.Next() {
			var u string
			if err := rows.Scan(&u); err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				rows.Close()
				return
			}
			candidates = append(candidates, u)
		}
		if err := rows.Err(); err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		rows.Close()

		if len(candidates) == 0 {
			http.Error(w, `{"error":{"code":"NO_CANDIDATE","message":"no active replacement candidate in team"}}`, http.StatusConflict)
			return
		}

		newReviewer := candidates[0]
		assigned[i] = newReviewer

		_, _ = db.Pool.Exec(ctx, `
			UPDATE pull_requests SET assigned_reviewers=$1 WHERE pull_request_id=$2
		`, assigned, req.PullRequestID)

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"pr":          models.PullRequest{PullRequestID: req.PullRequestID, AssignedReviewers: assigned},
			"replaced_by": newReviewer,
		}); err != nil {
			http.Error(w, "failed to encode response", http.StatusInternalServerError)
			return
		}
	}

	if !found {
		http.Error(w, `{"error":{"code":"NOT_ASSIGNED","message":"reviewer is not assigned to this PR"}}`, http.StatusConflict)
		return
	}
}
