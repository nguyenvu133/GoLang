CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS players (
    player_id TEXT PRIMARY KEY,
    account_id UUID UNIQUE REFERENCES accounts(id) ON DELETE CASCADE,
    display_name TEXT NOT NULL,
    room_id TEXT NOT NULL DEFAULT '',
    match_id TEXT NOT NULL DEFAULT '',
    x DOUBLE PRECISION NOT NULL DEFAULT 0,
    y DOUBLE PRECISION NOT NULL DEFAULT 0,
    z DOUBLE PRECISION NOT NULL DEFAULT 0,
    roty DOUBLE PRECISION NOT NULL DEFAULT 0,
    hp DOUBLE PRECISION NOT NULL DEFAULT 100,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS inventory (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    player_id TEXT NOT NULL REFERENCES players(player_id) ON DELETE CASCADE,
    item_name TEXT NOT NULL,
    quantity INTEGER NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (player_id, item_name)
);

CREATE INDEX IF NOT EXISTS idx_players_room_match
    ON players (room_id, match_id);
