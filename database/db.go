package database

import (
	"database/sql"
	"log"

	_ "github.com/mattn/go-sqlite3"
)

var DB *sql.DB

// InitDB opens the SQLite database, checks connectivity, and creates the application tables.
func InitDB(dbPath string) {
	var err error
	DB, err = sql.Open("sqlite3", dbPath+"?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	DB.SetMaxOpenConns(16)

	if err = DB.Ping(); err != nil {
		log.Fatalf("Database un-reachable: %v", err)
	}

	createTables()
}

// createTables creates the forum schema and message lookup indexes if they do not exist.
func createTables() {
	schema := `
		CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			nickname TEXT UNIQUE NOT NULL,
			age INTEGER NOT NULL,
			gender TEXT NOT NULL,
			first_name TEXT NOT NULL,
			last_name TEXT NOT NULL,
			email TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			expires_at DATETIME NOT NULL,
			FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);
		CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);

		CREATE TABLE IF NOT EXISTS categories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL
		);

		CREATE TABLE IF NOT EXISTS posts (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			title TEXT NOT NULL,
			content TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
		);

		CREATE TABLE IF NOT EXISTS post_categories (
			post_id TEXT NOT NULL,
			category_id INTEGER NOT NULL,
			PRIMARY KEY (post_id, category_id),
			FOREIGN KEY(post_id) REFERENCES posts(id) ON DELETE CASCADE,
			FOREIGN KEY(category_id) REFERENCES categories(id) ON DELETE CASCADE
		);

		CREATE TABLE IF NOT EXISTS comments (
			id TEXT PRIMARY KEY,
			post_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			content TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(post_id) REFERENCES posts(id) ON DELETE CASCADE,
			FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
		);

		CREATE TABLE IF NOT EXISTS messages (
			id TEXT PRIMARY KEY,
			sender_id TEXT NOT NULL,
			receiver_id TEXT NOT NULL,
			content TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(sender_id) REFERENCES users(id) ON DELETE CASCADE,
			FOREIGN KEY(receiver_id) REFERENCES users(id) ON DELETE CASCADE
		);

		CREATE INDEX IF NOT EXISTS idx_messages_sender_receiver_created
			ON messages(sender_id, receiver_id, created_at, id);
		CREATE INDEX IF NOT EXISTS idx_messages_receiver_sender_created
			ON messages(receiver_id, sender_id, created_at, id);
		CREATE INDEX IF NOT EXISTS idx_comments_post_created
			ON comments(post_id, created_at);
	`

	_, err := DB.Exec(schema)
	if err != nil {
		log.Fatalf("Failed to execute database schema: %v", err)
	}

	// The previous driver stored Go's time.String, including a zone name and monotonic clock.
	_, err = DB.Exec(`
		WITH legacy AS (
			SELECT id, expires_at, substr(expires_at, 20) AS suffix FROM sessions
			WHERE typeof(expires_at) = 'text' AND length(expires_at) > 19
		), offsets AS (
			SELECT id, expires_at, substr(suffix, instr(suffix, ' ') + 1, 5) AS zone_offset FROM legacy
		)
		UPDATE sessions
		SET expires_at = COALESCE(
			datetime(offsets.expires_at),
			datetime(substr(offsets.expires_at, 1, 19) || substr(zone_offset, 1, 3) || ':' || substr(zone_offset, 4, 2)),
			'1970-01-01 00:00:00'
		)
		FROM offsets
		WHERE sessions.id = offsets.id
	`)
	if err != nil {
		log.Fatalf("Failed to normalize existing session expiry times: %v", err)
	}
}
