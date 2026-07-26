package models

import "time"

type RegistrationInput struct {
	Nickname  string `json:"nickname"`
	Age       int    `json:"age"`
	Gender    string `json:"gender"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
	Password  string `json:"password"`
}

type User struct {
	ID        int        `json:"id"`
	Nickname  string     `json:"nickname"`
	Age       int        `json:"age,omitempty"`
	Gender    string     `json:"gender,omitempty"`
	FirstName string     `json:"first_name,omitempty"`
	LastName  string     `json:"last_name,omitempty"`
	Email     string     `json:"email,omitempty"`
	Online    bool       `json:"online,omitempty"`
	LastChat  *time.Time `json:"last_message_at,omitempty"`
}
