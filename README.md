# Real-time Leaderboard System

A real-time leaderboard backend API built with Go, PostgreSQL, and Redis. Users can register, submit scores for various games, and view rankings on global, per-game, and monthly leaderboards.

**Project URL:** https://roadmap.sh/projects/realtime-leaderboard-system

## Features

- **User Authentication** - Register and login with JWT-based authentication
- **Score Submission** - Submit scores for different games
- **Real-time Leaderboards** - Per-game, global, and monthly leaderboards using Redis sorted sets
- **User Rankings** - View your rank and score for any game
- **Role-based Access Control** - Admin-only endpoints for game management
- **Rate Limiting** - Protection against brute force and spam attacks

## Tech Stack

| Technology | Purpose |
|------------|---------|
| Go | Backend API |
| PostgreSQL | Persistent storage (users, games, score history) |
| Redis | Real-time leaderboards using Sorted Sets (ZSET) |
| Chi | HTTP router |
| JWT | Authentication tokens |
| bcrypt | Password hashing |

## Getting Started

### Prerequisites

- Go 1.23+
- PostgreSQL 14+
- Redis 7+

### Installation

1. Clone the repository:
   ```bash
   git clone https://github.com/4yushraman-jpg/realtime-leaderboard
   cd realtime-leaderboard
   ```

2. Install dependencies:
   ```bash
   go mod download
   ```

3. Create a `.env` file in the project root:
   ```env
   DATABASE_URL=postgres://user:password@localhost:5432/leaderboard?sslmode=disable
   REDIS_URL=redis://localhost:6379
   JWT_SECRET=your-secret-key-here
   ```

4. Run the database migrations:
   ```bash
   psql -d leaderboard -f internal/database/migrations/001_init.sql
   psql -d leaderboard -f internal/database/migrations/002_user_high_scores.sql
   ```

5. Start the server:
   ```bash
   go run cmd/api/main.go
   ```

The server will start on `http://localhost:8080`.

## API Endpoints

### Public Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/v1/users/register` | Register a new user |
| POST | `/api/v1/users/login` | Login and get JWT token |
| GET | `/api/v1/games` | List all games |
| GET | `/api/v1/games/{id}` | Get game by ID |

### Protected Endpoints (Requires JWT)

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/v1/scores` | Submit a score |
| GET | `/api/v1/games/{id}/leaderboard` | Get top 10 for a game |
| GET | `/api/v1/games/{id}/rank` | Get your rank in a game |
| GET | `/api/v1/leaderboard/global` | Get global top 10 |
| GET | `/api/v1/games/{id}/leaderboard/monthly` | Get monthly top 10 |

### Admin Endpoints (Requires Admin Role)

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/v1/games` | Create a new game |

## API Usage Examples

### Register a User

```bash
curl -X POST http://localhost:8080/api/v1/users/register \
  -H "Content-Type: application/json" \
  -d '{"username": "player1", "email": "player1@example.com", "password": "securepass123"}'
```

### Login

```bash
curl -X POST http://localhost:8080/api/v1/users/login \
  -H "Content-Type: application/json" \
  -d '{"email": "player1@example.com", "password": "securepass123"}'
```

Response:
```json
{"token": "eyJhbGciOiJIUzI1NiIs..."}
```

### Submit a Score

```bash
curl -X POST http://localhost:8080/api/v1/scores \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <your-token>" \
  -d '{"game_id": 1, "score": 1500}'
```

### Get Leaderboard

```bash
curl http://localhost:8080/api/v1/games/1/leaderboard \
  -H "Authorization: Bearer <your-token>"
```

Response:
```json
[
  {"rank": 1, "username": "player1", "score": 1500},
  {"rank": 2, "username": "player2", "score": 1200}
]
```

### Get Your Rank

```bash
curl http://localhost:8080/api/v1/games/1/rank \
  -H "Authorization: Bearer <your-token>"
```

### Get Monthly Leaderboard

```bash
curl "http://localhost:8080/api/v1/games/1/leaderboard/monthly?period=2026-03" \
  -H "Authorization: Bearer <your-token>"
```

## Database Schema

```
users
├── id (PK)
├── username (unique)
├── email (unique)
├── password_hash
├── role (user/admin)
└── created_at

games
├── id (PK)
├── name (unique)
├── description
└── created_at

score_history
├── id (PK)
├── user_id (FK -> users)
├── game_id (FK -> games)
├── score
└── achieved_at

user_high_scores
├── user_id (PK, FK -> users)
├── game_id (PK, FK -> games)
├── high_score
└── updated_at
```

## Redis Keys

| Key Pattern | Type | Description |
|-------------|------|-------------|
| `leaderboard:{gameID}` | Sorted Set | All-time leaderboard per game |
| `leaderboard:{gameID}:{YYYY-MM}` | Sorted Set | Monthly leaderboard |
| `leaderboard:global` | Sorted Set | Global leaderboard (sum of best scores) |

## Rate Limits

| Endpoint | Limit |
|----------|-------|
| `/users/register`, `/users/login` | 5 requests/minute per IP |
| All other endpoints | 100 requests/minute per IP |

## License

MIT
