package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"realtime-leaderboard/internal/models"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type UserHandler struct {
	DB        *pgxpool.Pool
	JWTSecret []byte
}

func (h *UserHandler) RegisterUserHandler(w http.ResponseWriter, r *http.Request) {
	var req models.RegisterUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	if req.Username == "" || req.Email == "" || req.Password == "" {
		http.Error(w, "Username, email or password mustn't be empty!", http.StatusBadRequest)
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	query := `INSERT INTO users (username, email, password_hash) VALUES ($1, $2, $3)`
	_, err = h.DB.Exec(r.Context(), query, req.Username, req.Email, string(hashedPassword))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			http.Error(w, "Username or email already exists", http.StatusConflict)
			return
		}
		http.Error(w, "Failed to save user", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "User added successfully",
	})
}

func (h *UserHandler) LoginUserHandler(w http.ResponseWriter, r *http.Request) {
	var req models.LoginUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	if req.Email == "" || req.Password == "" {
		http.Error(w, "Email or password mustn't be empty!", http.StatusBadRequest)
		return
	}

	var userID int
	var storedPassword string
	var role string

	query := `SELECT id, password_hash, role FROM users WHERE email = $1`
	err := h.DB.QueryRow(r.Context(), query, req.Email).Scan(&userID, &storedPassword, &role)
	if err == pgx.ErrNoRows {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	err = bcrypt.CompareHashAndPassword([]byte(storedPassword), []byte(req.Password))
	if err != nil {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	expirationTime := time.Now().Add(24 * time.Hour)
	claims := models.Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &claims)

	tokenString, err := token.SignedString(h.JWTSecret)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"token": tokenString,
	})
}
