package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"realtime-leaderboard/internal/middleware"
	"realtime-leaderboard/internal/models"
	"strconv"
	"time"

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

	upsertQuery := `
		INSERT INTO user_high_scores (user_id, game_id, high_score, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (user_id, game_id) 
		DO UPDATE SET high_score = GREATEST(user_high_scores.high_score, EXCLUDED.high_score),
		              updated_at = NOW()
		WHERE EXCLUDED.high_score > user_high_scores.high_score
	`
	_, err = h.DB.Exec(r.Context(), upsertQuery, claims.UserID, req.GameID, req.Score)
	if err != nil {
		log.Printf("Failed to update user_high_scores: %v", err)
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

	currentMonth := time.Now().Format("2006-01")
	monthlyRedisKey := fmt.Sprintf("leaderboard:%d:%s", req.GameID, currentMonth)

	err = h.Redis.ZAddArgs(r.Context(), monthlyRedisKey, redis.ZAddArgs{
		GT: true,
		Members: []redis.Z{
			{Score: float64(req.Score), Member: claims.UserID},
		},
	}).Err()
	if err != nil {
		log.Printf("Failed to update monthly Redis leaderboard: %v", err)
	}

	globalQuery := `SELECT COALESCE(SUM(high_score), 0) FROM user_high_scores WHERE user_id = $1`
	var globalScore float64
	err = h.DB.QueryRow(r.Context(), globalQuery, claims.UserID).Scan(&globalScore)
	if err != nil {
		log.Printf("Failed to calculate global score: %v", err)
	} else {
		err = h.Redis.ZAdd(r.Context(), "leaderboard:global", redis.Z{
			Score:  globalScore,
			Member: claims.UserID,
		}).Err()
		if err != nil {
			log.Printf("Failed to update global leaderboard: %v", err)
		}
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

func (h *ScoreHandler) GetUserRankHandler(w http.ResponseWriter, r *http.Request) {
	userContext := r.Context().Value(middleware.UserContextKey)
	claims, ok := userContext.(middleware.UserClaims)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	idStr := chi.URLParam(r, "id")
	gameID, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid game ID", http.StatusBadRequest)
		return
	}

	redisKey := fmt.Sprintf("leaderboard:%d", gameID)

	member := strconv.Itoa(claims.UserID)

	rank, err := h.Redis.ZRevRank(r.Context(), redisKey, member).Result()

	if err == redis.Nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "You haven't played this game yet!",
			"rank":    nil,
			"score":   nil,
		})
		return
	} else if err != nil {
		log.Printf("Redis error fetching rank: %v", err)
		http.Error(w, "Failed to fetch rank", http.StatusInternalServerError)
		return
	}

	score, err := h.Redis.ZScore(r.Context(), redisKey, member).Result()
	if err != nil && err != redis.Nil {
		log.Printf("Redis error fetching score: %v", err)
		http.Error(w, "Failed to fetch score", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"rank":  rank + 1,
		"score": score,
	})
}

func (h *ScoreHandler) GetGlobalLeaderboardHandler(w http.ResponseWriter, r *http.Request) {
	redisKey := "leaderboard:global"

	redisScores, err := h.Redis.ZRevRangeWithScores(r.Context(), redisKey, 0, 9).Result()
	if err != nil {
		log.Printf("Redis error fetching global leaderboard: %v", err)
		http.Error(w, "Failed to fetch global leaderboard", http.StatusInternalServerError)
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
		if rows.Scan(&id, &username) == nil {
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

func (h *ScoreHandler) GetMonthlyLeaderboardHandler(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	gameID, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid game ID", http.StatusBadRequest)
		return
	}

	period := r.URL.Query().Get("period")
	if period == "" {
		importTime := time.Now().Format("2006-01")
		period = importTime
	}

	redisKey := fmt.Sprintf("leaderboard:%d:%s", gameID, period)

	redisScores, err := h.Redis.ZRevRangeWithScores(r.Context(), redisKey, 0, 9).Result()
	if err != nil {
		log.Printf("Redis error fetching monthly leaderboard: %v", err)
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
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	userMap := make(map[int]string)
	for rows.Next() {
		var id int
		var username string
		if rows.Scan(&id, &username) == nil {
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
