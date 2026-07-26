package database

import (
	"database/sql"
	"errors"
	"strings"

	"real-time-forum/models"

	"golang.org/x/crypto/bcrypt"
)

func CreateUser(db *sql.DB, input models.RegistrationInput) (int, error) {
	normalized, err := validateRegistration(input)
	if err != nil {
		return 0, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(normalized.Password), bcrypt.DefaultCost)
	if err != nil {
		return 0, err
	}
	result, err := db.Exec(
		`INSERT INTO users (nickname, age, gender, first_name, last_name, email, password_hash) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		normalized.Nickname,
		normalized.Age,
		normalized.Gender,
		normalized.FirstName,
		normalized.LastName,
		strings.ToLower(normalized.Email),
		string(hash),
	)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	return int(id), err
}

func UserByIdentifier(db *sql.DB, identifier string) (models.User, string, error) {
	var user models.User
	var passwordHash string
	err := db.QueryRow(
		`SELECT id, nickname, age, gender, first_name, last_name, email, password_hash FROM users WHERE lower(nickname) = lower(?) OR lower(email) = lower(?)`,
		identifier,
		identifier,
	).Scan(&user.ID, &user.Nickname, &user.Age, &user.Gender, &user.FirstName, &user.LastName, &user.Email, &passwordHash)
	if errors.Is(err, sql.ErrNoRows) {
		return models.User{}, "", nil
	}
	return user, passwordHash, err
}

func UserByID(db *sql.DB, id int) (models.User, error) {
	var user models.User
	err := db.QueryRow(`SELECT id, nickname, age, gender, first_name, last_name, email FROM users WHERE id = ?`, id).
		Scan(&user.ID, &user.Nickname, &user.Age, &user.Gender, &user.FirstName, &user.LastName, &user.Email)
	return user, err
}

func PublicUser(user models.User) models.User {
	user.Email = ""
	user.Age = 0
	user.Gender = ""
	user.FirstName = ""
	user.LastName = ""
	return user
}

func PasswordMatches(passwordHash string, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)) == nil
}

func validateRegistration(input models.RegistrationInput) (models.RegistrationInput, error) {
	input.Nickname = strings.TrimSpace(input.Nickname)
	input.Gender = strings.TrimSpace(input.Gender)
	input.FirstName = strings.TrimSpace(input.FirstName)
	input.LastName = strings.TrimSpace(input.LastName)
	input.Email = strings.TrimSpace(input.Email)
	if input.Nickname == "" || input.Age <= 0 || input.Gender == "" || input.FirstName == "" || input.LastName == "" || input.Email == "" || len(input.Password) < 8 {
		return input, errors.New("all registration fields are required")
	}
	switch strings.ToLower(input.Gender) {
	case "male":
		input.Gender = "Male"
	case "female":
		input.Gender = "Female"
	default:
		return input, errors.New("gender must be Male or Female")
	}
	if !validEmail(input.Email) {
		return input, errors.New("invalid email")
	}
	return input, nil
}

func validEmail(value string) bool {
	if strings.ContainsAny(value, " \t\r\n") || strings.Count(value, "@") != 1 {
		return false
	}
	parts := strings.Split(value, "@")
	local, domain := parts[0], parts[1]
	if local == "" || domain == "" || !strings.Contains(domain, ".") {
		return false
	}
	for _, label := range strings.Split(domain, ".") {
		if label == "" {
			return false
		}
	}
	return true
}
