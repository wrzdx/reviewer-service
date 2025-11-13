package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"reviewer-service/app/db"
	"reviewer-service/app/models"

	"github.com/jackc/pgx/v5/pgconn"
)

func CreateTeamHandler(w http.ResponseWriter, r *http.Request) {
	var team models.Team
	if err := json.NewDecoder(r.Body).Decode(&team); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	_, err := db.Pool.Exec(context.Background(), `
		INSERT INTO teams(team_name) VALUES($1)
	`, team.TeamName)
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23505" { // уникальный ключ
			http.Error(w, `{"error":{"code":"TEAM_EXISTS","message":"team_name already exists"}}`, http.StatusBadRequest)
			return
		}
		log.Printf("Failed to create team: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	for _, member := range team.Members {
		_, _ = db.Pool.Exec(context.Background(), `
			INSERT INTO users(user_id, username, team_name, is_active)
			VALUES($1,$2,$3,$4)
			ON CONFLICT (user_id) DO UPDATE SET username=EXCLUDED.username, team_name=EXCLUDED.team_name, is_active=EXCLUDED.is_active
		`, member.UserID, member.Username, team.TeamName, member.IsActive)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]models.Team{"team": team})
}

func GetTeamHandler(w http.ResponseWriter, r *http.Request) {
	teamName := r.URL.Query().Get("team_name")
	if teamName == "" {
		http.Error(w, "team_name query param required", http.StatusBadRequest)
		return
	}

	var exists string
	err := db.Pool.QueryRow(context.Background(), "SELECT team_name FROM teams WHERE team_name=$1", teamName).Scan(&exists)
	if err != nil {
		http.Error(w, `{"error":{"code":"NOT_FOUND","message":"team not found"}}`, http.StatusNotFound)
		return
	}

	rows, err := db.Pool.Query(context.Background(), `
		SELECT user_id, username, is_active FROM users WHERE team_name=$1
	`, teamName)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	members := []models.TeamMember{}
	for rows.Next() {
		var m models.TeamMember
		if err := rows.Scan(&m.UserID, &m.Username, &m.IsActive); err != nil {
			continue
		}
		members = append(members, m)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models.Team{
		TeamName: teamName,
		Members:  members,
	})
}
