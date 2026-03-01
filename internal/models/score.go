package models

type SubmitScoreRequest struct {
	GameID int `json:"game_id"`
	Score  int `json:"score"`
}

type LeaderboardEntry struct {
	Rank     int     `json:"rank"`
	Username string  `json:"username"`
	Score    float64 `json:"score"`
}
