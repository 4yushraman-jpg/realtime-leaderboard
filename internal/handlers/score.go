package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"realtime-leaderboard/internal/middleware"
	"realtime-leaderboard/internal/models"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type ScoreHandler struct {
	DB    *pgxpool.Pool
	Redis *redis.Client
}

func (h *ScoreHandler) SubmitScoreHandler(w http.ResponseWriter, r *http.Request) {
	userContext := r.Context().Value(middleware.UserContextKey)
	claims, ok := userContext.(middleware.UserClaims)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req models.SubmitScoreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.GameID <= 0 || req.Score < 0 {
		http.Error(w, "Invalid game ID or score", http.StatusBadRequest)
		return
	}

	query := `INSERT INTO score_history (user_id, game_id, score) VALUES ($1, $2, $3)`
	_, err := h.DB.Exec(r.Context(), query, claims.UserID, req.GameID, req.Score)
	if err != nil {
		log.Printf("Failed to insert score into Postgres: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	redisKey := fmt.Sprintf("leaderboard:%d", req.GameID)
	err = h.Redis.ZAddArgs(r.Context(), redisKey, redis.ZAddArgs{
		GT: true,
		Members: []redis.Z{
			{Score: float64(req.Score), Member: claims.UserID},
		},
	}).Err()

	if err != nil {
		log.Printf("Failed to update Redis leaderboard: %v", err)
		http.Error(w, "Failed to update leaderboard", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Score submitted successfully!",
	})
}

func (h *ScoreHandler) GetLeaderboardHandler(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	gameID, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid game ID", http.StatusBadRequest)
		return
	}

	redisKey := fmt.Sprintf("leaderboard:%d", gameID)

	redisScores, err := h.Redis.ZRevRangeWithScores(r.Context(), redisKey, 0, 9).Result()
	if err != nil {
		log.Printf("Redis error fetching leaderboard: %v", err)
		http.Error(w, "Failed to fetch leaderboard", http.StatusInternalServerError)
		return
	}

	if len(redisScores) == 0 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]models.LeaderboardEntry{})
		return
	}

	var userIDs []int
	for _, z := range redisScores {
		userIDStr, _ := z.Member.(string)
		userID, _ := strconv.Atoi(userIDStr)
		userIDs = append(userIDs, userID)
	}

	query := `SELECT id, username FROM users WHERE id = ANY($1)`
	rows, err := h.DB.Query(r.Context(), query, userIDs)
	if err != nil {
		log.Printf("Postgres error fetching usernames: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	userMap := make(map[int]string)
	for rows.Next() {
		var id int
		var username string
		if err := rows.Scan(&id, &username); err == nil {
			userMap[id] = username
		}
	}

	leaderboard := []models.LeaderboardEntry{}
	for i, z := range redisScores {
		userIDStr, _ := z.Member.(string)
		userID, _ := strconv.Atoi(userIDStr)

		leaderboard = append(leaderboard, models.LeaderboardEntry{
			Rank:     i + 1,
			Username: userMap[userID],
			Score:    z.Score,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(leaderboard)
}
