package models

import "time"

type Game struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

type CreateGameRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}
