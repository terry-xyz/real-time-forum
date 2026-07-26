package models

import "time"

type Message struct {
	ID        int       `json:"id"`
	From      User      `json:"from"`
	To        User      `json:"to"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}
