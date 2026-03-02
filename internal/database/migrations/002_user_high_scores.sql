CREATE TABLE IF NOT EXISTS user_high_scores (
    user_id INT REFERENCES users(id) ON DELETE CASCADE,
    game_id INT REFERENCES games(id) ON DELETE CASCADE,
    high_score INT NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, game_id)
);

CREATE INDEX IF NOT EXISTS idx_user_high_scores_user_id ON user_high_scores(user_id);

CREATE INDEX IF NOT EXISTS idx_score_history_user_game ON score_history(user_id, game_id);
