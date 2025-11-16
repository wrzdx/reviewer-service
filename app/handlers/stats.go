package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"reviewer-service/app/db"
	"reviewer-service/app/models"
)

func GetAssignmentStatsHandler(w http.ResponseWriter, _ *http.Request) {
	ctx := context.Background()

	query := `
		SELECT 
			u.user_id, 
			u.username, 
			COUNT(reviewers.reviewer_id) AS assigned_pr_count
		FROM 
			users u
		LEFT JOIN 
			(SELECT unnest(assigned_reviewers) AS reviewer_id FROM pull_requests) AS reviewers 
            ON u.user_id = reviewers.reviewer_id
		GROUP BY 
			u.user_id, u.username
		ORDER BY 
			assigned_pr_count DESC, u.username ASC;
    `

	rows, err := db.Pool.Query(ctx, query)
	if err != nil {
		log.Printf("Failed to fetch assignment stats: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var stats []models.UserAssignmentStats
	for rows.Next() {
		var s models.UserAssignmentStats
		if err := rows.Scan(&s.UserID, &s.Username, &s.AssignedPRCount); err != nil {
			log.Printf("Failed to scan stats row: %v", err)
			continue
		}
		stats = append(stats, s)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(stats); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
}
