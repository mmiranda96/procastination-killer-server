package models

// UpdateFirebaseTokenRequest is a request for password reset
type UpdateFirebaseTokenRequest struct {
	FirebaseToken string `json:"firebase_token"`
}

// ResetPasswordRequest is a request for password reset
type ResetPasswordRequest struct {
	Token    string `json:"token"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// User is a user struct
type User struct {
	Name          string `json:"name"`
	Email         string `json:"email"`
	Password      string `json:"password,omitempty"`
	FirebaseToken string `json:"firebase_token,omitempty"`
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
