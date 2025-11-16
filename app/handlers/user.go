package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"reviewer-service/app/db"
	"reviewer-service/app/models"

	"github.com/jackc/pgx/v5"
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

func deactivateUsers(ctx context.Context, tx pgx.Tx, userIDs []string) ([]string, error) {
	deactivateQuery := `UPDATE users SET is_active = FALSE WHERE user_id = ANY($1) AND is_active = TRUE RETURNING user_id`
	rows, err := tx.Query(ctx, deactivateQuery, userIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to execute deactivate query: %w", err)
	}
	defer rows.Close()

	var deactivatedUsers []string
	for rows.Next() {
		var userID string
		if err = rows.Scan(&userID); err != nil {
			log.Printf("Error scanning deactivated user ID: %v", err)
			continue
		}
		deactivatedUsers = append(deactivatedUsers, userID)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error during iteration over deactivated users: %w", err)
	}
	return deactivatedUsers, nil
}

func findOpenPRs(ctx context.Context, tx pgx.Tx, authorIDs []string) ([]models.ReassignmentDetail, error) {
	findPRsQuery := `
        SELECT pull_request_id, author_id FROM pull_requests 
        WHERE author_id = ANY($1) AND status = 'OPEN'
    `
	rows, err := tx.Query(ctx, findPRsQuery, authorIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to execute find PRs query: %w", err)
	}
	defer rows.Close()

	var details []models.ReassignmentDetail
	for rows.Next() {
		var prID, authorID string
		if err = rows.Scan(&prID, &authorID); err != nil {
			log.Printf("Error scanning open PR row: %v", err)
			continue
		}
		details = append(details, models.ReassignmentDetail{
			PullRequestID: prID,
			OldAuthorID:   authorID,
		})
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error during iteration over open PRs: %w", err)
	}
	return details, nil
}

func ProcessUserDeactivationHandler(w http.ResponseWriter, r *http.Request) {
	var req models.DeactivateUsersRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		log.Printf("Failed to start transaction: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	defer func() {
		if rbe := tx.Rollback(ctx); rbe != nil && !errors.Is(rbe, pgx.ErrTxClosed) {
			log.Printf("Rollback error in ProcessUserDeactivationHandler: %v", rbe)
		}
	}()

	deactivatedUsers, err := deactivateUsers(ctx, tx, req.UserIDs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	reassignmentDetails, err := findOpenPRs(ctx, tx, deactivatedUsers)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for i, detail := range reassignmentDetails {
		newReviewerID, err := AssignNewReviewer(ctx, tx, detail.PullRequestID)
		if err != nil {
			log.Printf("Failed to reassign PR %s: %v", detail.PullRequestID, err)
			http.Error(w, fmt.Sprintf("failed to reassign PR %s", detail.PullRequestID), http.StatusBadRequest)
			return
		}
		reassignmentDetails[i].NewReviewerID = newReviewerID
	}

	if err := tx.Commit(ctx); err != nil {
		log.Printf("Failed to commit transaction: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	response := models.DeactivationResponse{
		Status:              "completed",
		DeactivatedUsers:    deactivatedUsers,
		ReassignedPRsCount:  len(reassignmentDetails),
		ReassignmentDetails: reassignmentDetails,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response in ProcessUserDeactivationHandler: %v", err)
	}
}

func AssignNewReviewer(ctx context.Context, tx pgx.Tx, prID string) (string, error) {
	var currentAuthorID, teamName string
	err := tx.QueryRow(ctx, `
		SELECT p.author_id, u.team_name 
		FROM pull_requests p
		JOIN users u ON p.author_id = u.user_id
		WHERE p.pull_request_id = $1
	`, prID).Scan(&currentAuthorID, &teamName)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("PR or author not found for PR ID %s", prID)
		}
		return "", fmt.Errorf("failed to fetch PR author info: %w", err)
	}

	var newReviewerID string
	err = tx.QueryRow(ctx, `
		SELECT user_id FROM users 
		WHERE team_name = $1 AND is_active = TRUE AND user_id != $2
		ORDER BY RANDOM() LIMIT 1 -- Выбираем случайного активного пользователя
	`, teamName, currentAuthorID).Scan(&newReviewerID)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("no suitable active reviewer found in team %s", teamName)
		}
		return "", fmt.Errorf("failed to find a new reviewer: %w", err)
	}

	updateQuery := `
		UPDATE pull_requests 
		SET assigned_reviewers = ARRAY[$1::text] 
		WHERE pull_request_id = $2
	`
	_, err = tx.Exec(ctx, updateQuery, newReviewerID, prID)
	if err != nil {
		return "", fmt.Errorf("failed to update PR assigned_reviewers: %w", err)
	}

	return newReviewerID, nil
}
