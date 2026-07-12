package main

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	_ "github.com/lib/pq"
)

type PostgresStore struct {
	db *sql.DB
	mu sync.RWMutex
}

func NewPostgresStore(connString string) (*PostgresStore, error) {
	if connString == "" {
		connString = os.Getenv("DATABASE_URL")
	}
	if connString == "" {
		connString = "postgres://postgres:postgres@localhost:5432/gamegodot?sslmode=disable"
	}

	db, err := sql.Open("postgres", connString)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}

	store := &PostgresStore{db: db}
	if err := store.InitSchema(); err != nil {
		return nil, err
	}

	return store, nil
}

func (s *PostgresStore) InitSchema() error {
	queries := []string{
		"CREATE EXTENSION IF NOT EXISTS pgcrypto",
		`CREATE TABLE IF NOT EXISTS accounts (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			username TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS players (
			player_id TEXT PRIMARY KEY,
			account_id UUID UNIQUE NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
			display_name TEXT NOT NULL,
			room_id TEXT NOT NULL DEFAULT '',
			match_id TEXT NOT NULL DEFAULT '',
			x DOUBLE PRECISION NOT NULL DEFAULT 0,
			y DOUBLE PRECISION NOT NULL DEFAULT 0,
			z DOUBLE PRECISION NOT NULL DEFAULT 0,
			roty DOUBLE PRECISION NOT NULL DEFAULT 0,
			hp DOUBLE PRECISION NOT NULL DEFAULT 100,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS inventory (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			player_id TEXT NOT NULL REFERENCES players(player_id) ON DELETE CASCADE,
			item_name TEXT NOT NULL,
			quantity INTEGER NOT NULL DEFAULT 0,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_players_room_match ON players(room_id, match_id)`,
	}

	for _, query := range queries {
		if _, err := s.db.Exec(query); err != nil {
			return err
		}
	}
	return nil
}

func (s *PostgresStore) LoginOrCreateAccount(username string, password string) (PlayerState, []InventoryItem, error) {
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	if username == "" || password == "" {
		return PlayerState{}, nil, fmt.Errorf("username and password are required")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return PlayerState{}, nil, err
	}
	defer tx.Rollback()

	var accountID string
	if err := tx.QueryRow(`SELECT id FROM accounts WHERE username = $1 AND password_hash = $2`, username, password).Scan(&accountID); err != nil {
		if err == sql.ErrNoRows {
			if _, err = tx.Exec(`INSERT INTO accounts (username, password_hash) VALUES ($1, $2) ON CONFLICT (username) DO NOTHING`, username, password); err != nil {
				return PlayerState{}, nil, err
			}
			if err := tx.QueryRow(`SELECT id FROM accounts WHERE username = $1 AND password_hash = $2`, username, password).Scan(&accountID); err != nil {
				return PlayerState{}, nil, err
			}
		} else {
			return PlayerState{}, nil, err
		}
	}

	playerID := fmt.Sprintf("player_%d", time.Now().UnixNano())
	var state PlayerState
	if err := tx.QueryRow(`SELECT player_id, display_name, room_id, match_id, x, y, z, roty, hp FROM players WHERE account_id = $1`, accountID).Scan(
		&state.ID,
		&state.Name,
		&state.RoomID,
		&state.MatchID,
		&state.X,
		&state.Y,
		&state.Z,
		&state.RotY,
		&state.HP,
	); err != nil {
		if err == sql.ErrNoRows {
			state.ID = playerID
			state.Name = username
			state.HP = 100
			_, err = tx.Exec(
				`INSERT INTO players (player_id, account_id, display_name, room_id, match_id, x, y, z, roty, hp) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
				state.ID,
				accountID,
				state.Name,
				state.RoomID,
				state.MatchID,
				state.X,
				state.Y,
				state.Z,
				state.RotY,
				state.HP,
			)
			if err != nil {
				return PlayerState{}, nil, err
			}
		} else {
			return PlayerState{}, nil, err
		}
	}

	inventory, err := s.GetInventory(state.ID, tx)
	if err != nil {
		return PlayerState{}, nil, err
	}

	if err := tx.Commit(); err != nil {
		return PlayerState{}, nil, err
	}

	return state, inventory, nil
}

func (s *PostgresStore) GetInventory(playerID string, tx *sql.Tx) ([]InventoryItem, error) {
	rows, err := tx.Query(`SELECT item_name, quantity FROM inventory WHERE player_id = $1`, playerID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()

	items := make([]InventoryItem, 0)
	for rows.Next() {
		var item InventoryItem
		if err := rows.Scan(&item.Name, &item.Quantity); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(items) == 0 {
		_, err = tx.Exec(`INSERT INTO inventory (player_id, item_name, quantity) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, playerID, "sword", 1)
		if err != nil {
			return nil, err
		}
		_, err = tx.Exec(`INSERT INTO inventory (player_id, item_name, quantity) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, playerID, "potion", 2)
		if err != nil {
			return nil, err
		}
		return s.GetInventory(playerID, tx)
	}

	return items, nil
}

func (s *PostgresStore) UpsertPlayerState(state PlayerState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if state.ID == "" {
		return fmt.Errorf("player id is required")
	}

	_, err := s.db.Exec(
		`INSERT INTO players (player_id, account_id, display_name, room_id, match_id, x, y, z, roty, hp, updated_at)
		VALUES ($1, NULL, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
		ON CONFLICT (player_id)
		DO UPDATE SET display_name = EXCLUDED.display_name,
			room_id = EXCLUDED.room_id,
			match_id = EXCLUDED.match_id,
			x = EXCLUDED.x,
			y = EXCLUDED.y,
			z = EXCLUDED.z,
			roty = EXCLUDED.roty,
			hp = EXCLUDED.hp,
			updated_at = NOW()`,
		state.ID,
		state.Name,
		state.RoomID,
		state.MatchID,
		state.X,
		state.Y,
		state.Z,
		state.RotY,
		state.HP,
	)
	return err
}

func (s *PostgresStore) ApplyDamage(targetID string, damage float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`UPDATE players SET hp = GREATEST(hp - $1, 0), updated_at = NOW() WHERE player_id = $2`, damage, targetID)
	return err
}

func (s *PostgresStore) Snapshot(roomID string, matchID string) ([]PlayerState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`SELECT player_id, display_name, room_id, match_id, x, y, z, roty, hp FROM players WHERE room_id = $1 AND match_id = $2 ORDER BY updated_at DESC`, roomID, matchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	players := make([]PlayerState, 0)
	for rows.Next() {
		var state PlayerState
		if err := rows.Scan(&state.ID, &state.Name, &state.RoomID, &state.MatchID, &state.X, &state.Y, &state.Z, &state.RotY, &state.HP); err != nil {
			return nil, err
		}
		players = append(players, state)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return players, nil
}
