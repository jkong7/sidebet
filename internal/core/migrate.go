package core

import (
	"database/sql"
	"fmt"
)

var migrations = []string{
	`ALTER TABLE groups ADD COLUMN kind TEXT NOT NULL DEFAULT 'friends';
	 ALTER TABLE groups ADD COLUMN domains TEXT NOT NULL DEFAULT '';
	 ALTER TABLE users ADD COLUMN email TEXT;
	 CREATE UNIQUE INDEX IF NOT EXISTS users_email ON users (email) WHERE email IS NOT NULL;
	 ALTER TABLE members ADD COLUMN role TEXT NOT NULL DEFAULT 'member';
	 CREATE TABLE IF NOT EXISTS sessions (
	     token_hash TEXT PRIMARY KEY,
	     user_id    INTEGER NOT NULL REFERENCES users(id),
	     created_at INTEGER NOT NULL
	 );
	 CREATE TABLE IF NOT EXISTS verifications (
	     email      TEXT PRIMARY KEY,
	     code_hash  TEXT    NOT NULL,
	     expires_at INTEGER NOT NULL,
	     attempts   INTEGER NOT NULL DEFAULT 0,
	     sent_at    INTEGER NOT NULL
	 );
	 CREATE TABLE IF NOT EXISTS reports (
	     id         INTEGER PRIMARY KEY,
	     market_id  INTEGER NOT NULL REFERENCES markets(id),
	     user_id    INTEGER NOT NULL REFERENCES users(id),
	     reason     TEXT    NOT NULL,
	     created_at INTEGER NOT NULL,
	     UNIQUE (market_id, user_id)
	 );`,
}

func migrate(db *sql.DB) error {
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}
	for i := version; i < len(migrations); i++ {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, i+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
