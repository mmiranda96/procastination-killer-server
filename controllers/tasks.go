package controllers

import (
	"database/sql"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/mmiranda96/procastination-killer-server/models"
)

// Task is a contrller for tasks
type Task struct {
	DB *sql.DB
}

// GetTasks returns all tasks from a user
func (c *Task) GetTasks(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value(userCtxKey).(*models.User)

	tasks, err := c.getTasksFromDB(user.Email)
	if err != nil {
		log.Println(err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	bytes, _ := json.Marshal(tasks)
	w.Write(bytes)
}

// CreateTask creates a new task for a user
func (c *Task) CreateTask(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value(userCtxKey).(*models.User)

	body, _ := ioutil.ReadAll(r.Body)
	task := &models.Task{}
	if err := json.Unmarshal(body, task); err != nil {
		log.Println(err)
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	if err := c.createTaskInDB(user.Email, task); err != nil {
		log.Println(err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

func (c *Task) getTasksFromDB(email string) ([]*models.Task, error) {
	const query = `
	SELECT tasks.id, title, description, due
	FROM tasks
	JOIN users
	ON tasks.user_id = users.id
	WHERE users.email = $1;
	`
	rows, err := c.DB.Query(query, email)
	if err != nil {
		return nil, err
	}

	var description sql.NullString
	tasks := make([]*models.Task, 0)
	for rows.Next() {
		task := &models.Task{}
		var due time.Time
		if err := rows.Scan(&task.ID, &task.Title, &description, &due); err != nil {
			return nil, err
		}

		const subtasksQuery = `
		SELECT description
		FROM subtasks
		JOIN users
		ON subtasks.user_id = users.id
		WHERE users.email = $1
		AND subtasks.task_id = $2;
		`
		subtasksRows, err := c.DB.Query(subtasksQuery, email, task.ID)
		if err != nil {
			return nil, err
		}

		subtasks := make([]string, 0)
		for subtasksRows.Next() {
			var subtask string
			if err := subtasksRows.Scan(&subtask); err != nil {
				return nil, err
			}

			subtasks = append(subtasks, subtask)
		}

		task.Description = description.String
		task.Subtasks = subtasks
		task.Due = due.Unix()
		tasks = append(tasks, task)
	}

	return tasks, nil
}

func (c *Task) createTaskInDB(email string, task *models.Task) error {
	userID, err := c.getUserID(email)
	if err != nil {
		return err
	}

	const query = `
	INSERT INTO tasks(user_id, title, description, due)
	VALUES ($1, $2, $3, $4)
	RETURNING id;
	`
	due := time.Unix(task.Due, 0)
	var taskID int
	if err := c.DB.QueryRow(query, userID, task.Title, task.Description, due).Scan(&taskID); err != nil {
		return err
	}

	for _, subtask := range task.Subtasks {
		const querySubtasks = `
		INSERT INTO subtasks(user_id, task_id, description)
		VALUES ($1, $2, $3)
		`
		if _, err := c.DB.Exec(querySubtasks, userID, int(taskID), subtask); err != nil {
			return err
		}
	}

	return nil
}

func (c *Task) getUserID(email string) (int, error) {
	const query = `
	SELECT id
	FROM users
	WHERE email = $1
	`
	var id int
	if err := c.DB.QueryRow(query, email).Scan(&id); err != nil {
		return 0, err
	}

	return id, nil
}
