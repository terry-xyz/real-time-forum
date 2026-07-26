package database

import (
	"database/sql"
	"embed"
	"fmt"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const DefaultDBPath = "real-time-forum.db"

//go:embed schema.sql
var schemaFiles embed.FS

func InitDB(path string) (*sql.DB, error) {
	if strings.TrimSpace(path) == "" {
		path = DefaultDBPath
	}
	db, err := sql.Open("sqlite3", sqliteDSN(path))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	schemaSQL, err := schemaFiles.ReadFile("schema.sql")
	if err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(string(schemaSQL)); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func sqliteDSN(path string) string {
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return path + separator + "_foreign_keys=on&_busy_timeout=5000"
}

func parseDBTime(value string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05"} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid database timestamp %q", value)
}
