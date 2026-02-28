package middleware

import (
	"context"
	"fmt"
	"net/http"
	"realtime-leaderboard/internal/models"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const UserContextKey contextKey = "userContext"

type UserClaims struct {
	UserID int
	Role   string
}

func AuthMiddleware(jwtSecret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "Empty auth header", http.StatusUnauthorized)
				return
			}

			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || parts[0] != "Bearer" {
				http.Error(w, "Invalid auth header format", http.StatusUnauthorized)
				return
			}

			tokenString := parts[1]

			claims := &models.Claims{}

			token, err := jwt.ParseWithClaims(
				tokenString,
				claims,
				func(token *jwt.Token) (interface{}, error) {
					if token.Method != jwt.SigningMethodHS256 {
						return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
					}
					return jwtSecret, nil
				},
			)

			if err != nil || !token.Valid {
				http.Error(w, "Invalid token", http.StatusUnauthorized)
				return
			}

			UserCtxPayload := UserClaims{
				UserID: claims.UserID,
				Role:   claims.Role,
			}

			ctx := context.WithValue(r.Context(), UserContextKey, UserCtxPayload)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func AdminOnlyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctxValue := r.Context().Value(UserContextKey)
		if ctxValue == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		claims, ok := ctxValue.(UserClaims)
		if !ok {
			http.Error(w, "Server error", http.StatusInternalServerError)
			return
		}

		if claims.Role != "admin" {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}
