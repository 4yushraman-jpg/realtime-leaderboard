package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"realtime-leaderboard/internal/models"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type GameHandler struct {
	DB *pgxpool.Pool
}

func (h *GameHandler) CreateGameHandler(w http.ResponseWriter, r *http.Request) {
	var req models.CreateGameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	req.Name = strings.ToLower(strings.TrimSpace(req.Name))
	if req.Name == "" {
		http.Error(w, "Name mustn't be empty!", http.StatusBadRequest)
		return
	}

	query := `INSERT INTO games (name, description) VALUES ($1, $2)`
	_, err := h.DB.Exec(r.Context(), query, req.Name, req.Description)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			http.Error(w, "Game already exists", http.StatusConflict)
			return
		}
		log.Printf("Failed to insert game into DB: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Game created successfully!",
	})
}

func (h *GameHandler) GetGamesHandler(w http.ResponseWriter, r *http.Request) {
	query := `SELECT id, name, description, created_at FROM games`

	rows, err := h.DB.Query(r.Context(), query)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	games := []models.Game{}
	for rows.Next() {
		var g models.Game
		if err := rows.Scan(&g.ID, &g.Name, &g.Description, &g.CreatedAt); err != nil {
			http.Error(w, "Failed to fetch games", http.StatusInternalServerError)
			return
		}
		games = append(games, g)
	}

	if rows.Err() != nil {
		http.Error(w, "Error iterating games", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(games)
}

func (h *GameHandler) GetGameByIDHandler(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid game ID", http.StatusBadRequest)
		return
	}

	query := `SELECT id, name, description, created_at FROM games WHERE id = $1`
	var game models.Game
	err = h.DB.QueryRow(r.Context(), query, id).Scan(&game.ID, &game.Name, &game.Description, &game.CreatedAt)
	if err == pgx.ErrNoRows {
		http.Error(w, "Game not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(game)
}
