package main

import (
	"log"
	"net/http"
	"reviewer-service/app/db"
	"reviewer-service/app/handlers"
	"time"

	"github.com/gorilla/mux"
)

func main() {
	if err := db.Init(); err != nil {
		log.Fatal("Failed to connect to DB:", err)
	}
	defer db.Close()

	r := mux.NewRouter()

	// Team endpoints
	teamRouter := r.PathPrefix("/team").Subrouter()
	teamRouter.HandleFunc("/add", handlers.CreateTeamHandler).Methods("POST")
	teamRouter.HandleFunc("/get", handlers.GetTeamHandler).Methods("GET")

	// User endpoints
	userRouter := r.PathPrefix("/users").Subrouter()
	userRouter.HandleFunc("/setIsActive", handlers.SetUserActiveHandler).Methods("POST")
	userRouter.HandleFunc("/getReview", handlers.GetUserPRsHandler).Methods("GET")

	// PullRequest endpoints
	prRouter := r.PathPrefix("/pullRequest").Subrouter()
	prRouter.HandleFunc("/create", handlers.CreatePRHandler).Methods("POST")
	prRouter.HandleFunc("/merge", handlers.MergePRHandler).Methods("POST")
	prRouter.HandleFunc("/reassign", handlers.ReassignPRHandler).Methods("POST")

	// Stats endpoints
	r.HandleFunc("/stats/assignments", handlers.GetAssignmentStatsHandler).Methods("GET")

	log.Println("Server starting on :8080")
	srv := &http.Server{
		Addr:         ":8080",
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	log.Println("Server starting on :8080")
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		db.Close()
		log.Printf("server error: %v", err)
	}
}
