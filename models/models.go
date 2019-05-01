package models

// User is a user struct
type User struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password,omitempty"`
}

// Task is a task struct
type Task struct {
	ID          int        `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Due         int64      `json:"due"`
	Subtasks    []string   `json:"subtasks"`
	Coords      [2]float64 `json:"coords"`
}
