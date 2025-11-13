package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"reviewer-service/app/db"
	"reviewer-service/app/models"

	"github.com/jackc/pgx/v5/pgconn"
)

func SetUserActiveHandler(w http.ResponseWriter, r *http.Request) {
	var req models.SetUserActiveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	commandTag, err := db.Pool.Exec(context.Background(), `
		UPDATE users SET is_active=$1 WHERE user_id=$2
	`, req.IsActive, req.UserID)

	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok {
			http.Error(w, pgErr.Message, http.StatusInternalServerError)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if commandTag.RowsAffected() == 0 {
		http.Error(w, `{"error":{"code":"NOT_FOUND","message":"user not found"}}`, http.StatusNotFound)
		return
	}

	var username, teamName string
	var isActive bool
	err = db.Pool.QueryRow(context.Background(), `
		SELECT username, team_name, is_active FROM users WHERE user_id=$1
	`, req.UserID).Scan(&username, &teamName, &isActive)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"user": map[string]any{
			"user_id":   req.UserID,
			"username":  username,
			"team_name": teamName,
			"is_active": isActive,
		},
	}); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
}

func GetUserPRsHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		http.Error(w, "user_id query param required", http.StatusBadRequest)
		return
	}

	rows, err := db.Pool.Query(context.Background(), `
		SELECT pull_request_id, pull_request_name, author_id, status
		FROM pull_requests
		WHERE $1 = ANY(assigned_reviewers)
	`, userID)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	prs := []models.PullRequestShort{}
	for rows.Next() {
		var pr models.PullRequestShort
		if err := rows.Scan(&pr.PullRequestID, &pr.PullRequestName, &pr.AuthorID, &pr.Status); err != nil {
			continue
		}
		prs = append(prs, pr)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"user_id":       userID,
		"pull_requests": prs,
	}); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
}
