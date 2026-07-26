package database

import "database/sql"

type Category struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func ListCategories(db *sql.DB) ([]Category, error) {
	rows, err := db.Query(`SELECT id, name FROM categories ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	categories := make([]Category, 0)
	for rows.Next() {
		var category Category
		if err := rows.Scan(&category.ID, &category.Name); err != nil {
			return nil, err
		}
		categories = append(categories, category)
	}
	return categories, rows.Err()
}
