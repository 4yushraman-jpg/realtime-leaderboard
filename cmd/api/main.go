package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"realtime-leaderboard/internal/database"
	"realtime-leaderboard/internal/handlers"
	"realtime-leaderboard/internal/middleware"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httprate"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Fatal(err)
	}

	log.Println("Connecting to the database...")
	dbPool, err := database.ConnectDB()
	if err != nil {
		log.Fatal(err)
	}
	defer dbPool.Close()
	log.Println("Database connection established.")

	log.Println("Connecting to Redis...")
	redisClient, err := database.ConnectRedis()
	if err != nil {
		log.Fatal(err)
	}
	defer redisClient.Close()
	log.Println("Redis connection established")

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET environment variable is not set")
	}

	userHandler := handlers.UserHandler{
		DB:        dbPool,
		JWTSecret: []byte(jwtSecret),
	}

	gameHandler := handlers.GameHandler{
		DB: dbPool,
	}

	scoreHandler := handlers.ScoreHandler{
		DB:    dbPool,
		Redis: redisClient,
	}

	r := chi.NewRouter()

	r.Use(httprate.LimitByIP(100, time.Minute))

	r.Route("/api/v1", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(httprate.LimitByIP(5, time.Minute))
			r.Post("/users/register", userHandler.RegisterUserHandler)
			r.Post("/users/login", userHandler.LoginUserHandler)
		})

		r.Get("/games", gameHandler.GetGamesHandler)
		r.Get("/games/{id}", gameHandler.GetGameByIDHandler)

		r.Group(func(r chi.Router) {
			r.Use(middleware.AuthMiddleware([]byte(jwtSecret)))
			r.Post("/scores", scoreHandler.SubmitScoreHandler)
			r.Get("/games/{id}/leaderboard", scoreHandler.GetLeaderboardHandler)
			r.Get("/games/{id}/rank", scoreHandler.GetUserRankHandler)
			r.Get("/leaderboard/global", scoreHandler.GetGlobalLeaderboardHandler)
			r.Get("/games/{id}/leaderboard/monthly", scoreHandler.GetMonthlyLeaderboardHandler)

			r.Group(func(r chi.Router) {
				r.Use(middleware.AdminOnlyMiddleware)
				r.Post("/games", gameHandler.CreateGameHandler)
			})
		})
	})

	srv := &http.Server{
		Addr:    ":8080",
		Handler: r,
	}

	go func() {
		log.Printf("Starting server on port %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exiting gracefully")
}
