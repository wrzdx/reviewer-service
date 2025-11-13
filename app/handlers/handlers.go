package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"reviewer-service/app/db"

	"github.com/jackc/pgx/v5/pgconn"
)

// ===== Models =====
type User struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	TeamName string `json:"team_name"`
	IsActive bool   `json:"is_active"`
}

type TeamMember struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	IsActive bool   `json:"is_active"`
}

type Team struct {
	TeamName string       `json:"team_name"`
	Members  []TeamMember `json:"members"`
}

type PullRequest struct {
	PullRequestID     string   `json:"pull_request_id"`
	PullRequestName   string   `json:"pull_request_name"`
	AuthorID          string   `json:"author_id"`
	Status            string   `json:"status"` // OPEN или MERGED
	AssignedReviewers []string `json:"assigned_reviewers"`
}

// ===== Handlers =====

// Team
func CreateTeamHandler(w http.ResponseWriter, r *http.Request) {
	var team Team
	if err := json.NewDecoder(r.Body).Decode(&team); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Создаем команду
	_, err := db.Pool.Exec(context.Background(), `
		INSERT INTO teams(team_name) VALUES($1)
	`, team.TeamName)
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23505" { // уникальный ключ
			http.Error(w, `{"error":{"code":"TEAM_EXISTS","message":"team_name already exists"}}`, http.StatusBadRequest)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Создаем участников
	for _, member := range team.Members {
		_, _ = db.Pool.Exec(context.Background(), `
			INSERT INTO users(user_id, username, team_name, is_active)
			VALUES($1,$2,$3,$4)
			ON CONFLICT (user_id) DO UPDATE SET username=EXCLUDED.username, team_name=EXCLUDED.team_name, is_active=EXCLUDED.is_active
		`, member.UserID, member.Username, team.TeamName, member.IsActive)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]Team{"team": team})
}

func GetTeamHandler(w http.ResponseWriter, r *http.Request) {
	teamName := r.URL.Query().Get("team_name")
	if teamName == "" {
		http.Error(w, "team_name query param required", http.StatusBadRequest)
		return
	}

	// Проверяем команду
	var exists string
	err := db.Pool.QueryRow(context.Background(), "SELECT team_name FROM teams WHERE team_name=$1", teamName).Scan(&exists)
	if err != nil {
		http.Error(w, `{"error":{"code":"NOT_FOUND","message":"team not found"}}`, http.StatusNotFound)
		return
	}

	// Получаем участников
	rows, err := db.Pool.Query(context.Background(), `
		SELECT user_id, username, is_active FROM users WHERE team_name=$1
	`, teamName)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	members := []TeamMember{}
	for rows.Next() {
		var m TeamMember
		if err := rows.Scan(&m.UserID, &m.Username, &m.IsActive); err != nil {
			continue
		}
		members = append(members, m)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Team{
		TeamName: teamName,
		Members:  members,
	})
}

type SetUserActiveRequest struct {
	UserID   string `json:"user_id"`
	IsActive bool   `json:"is_active"`
}

// User
func SetUserActiveHandler(w http.ResponseWriter, r *http.Request) {
	var req SetUserActiveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Обновляем is_active
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

	// Возвращаем обновленного пользователя
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
	json.NewEncoder(w).Encode(map[string]interface{}{
		"user": map[string]interface{}{
			"user_id":   req.UserID,
			"username":  username,
			"team_name": teamName,
			"is_active": isActive,
		},
	})
}

type PullRequestShort struct {
	PullRequestID   string `json:"pull_request_id"`
	PullRequestName string `json:"pull_request_name"`
	AuthorID        string `json:"author_id"`
	Status          string `json:"status"`
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

	prs := []PullRequestShort{}
	for rows.Next() {
		var pr PullRequestShort
		if err := rows.Scan(&pr.PullRequestID, &pr.PullRequestName, &pr.AuthorID, &pr.Status); err != nil {
			continue
		}
		prs = append(prs, pr)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"user_id":       userID,
		"pull_requests": prs,
	})
}

// PullRequest
func CreatePRHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message":"CreatePRHandler works"}`))
}

func MergePRHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message":"MergePRHandler works"}`))
}

func ReassignPRHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message":"ReassignPRHandler works"}`))
}
