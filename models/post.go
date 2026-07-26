package models

import "time"

type Post struct {
	ID         int       `json:"id"`
	Title      string    `json:"title"`
	Content    string    `json:"content"`
	Author     User      `json:"author"`
	Categories []string  `json:"categories"`
	CreatedAt  time.Time `json:"created_at"`
}
