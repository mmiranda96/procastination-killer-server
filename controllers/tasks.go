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

// UpdateTask updates an existing task for a user
func (c *Task) UpdateTask(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value(userCtxKey).(*models.User)

	body, _ := ioutil.ReadAll(r.Body)
	task := &models.Task{}
	if err := json.Unmarshal(body, task); err != nil {
		log.Println(err)
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	if err := c.updateTaskInDB(user.Email, task); err != nil {
		log.Println(err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

func (c *Task) getTasksFromDB(email string) ([]*models.Task, error) {
	const query = `
	SELECT tasks.id, title, description, due, latitude, longitude
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
		var (
			due       time.Time
			latitude  sql.NullFloat64
			longitude sql.NullFloat64
		)
		if err := rows.Scan(&task.ID, &task.Title, &description, &due, &latitude, &longitude); err != nil {
			return nil, err
		}

		const subtasksQuery = `
		SELECT subtasks.description
		FROM subtasks
		JOIN tasks
		ON subtasks.task_id = tasks.id
		WHERE subtasks.task_id = $1;
		`
		subtasksRows, err := c.DB.Query(subtasksQuery, task.ID)
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
		task.Coords = [2]float64{latitude.Float64, longitude.Float64}
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
	INSERT INTO tasks(user_id, title, description, due, latitude, longitude)
	VALUES ($1, $2, $3, $4, $5, $6)
	RETURNING id;
	`
	due := time.Unix(task.Due, 0)
	var taskID int
	if err := c.DB.QueryRow(query, userID, task.Title, task.Description, due, task.Coords[0], task.Coords[1]).Scan(&taskID); err != nil {
		return err
	}

	for _, subtask := range task.Subtasks {
		const querySubtasks = `
		INSERT INTO subtasks(task_id, description)
		VALUES ($1, $2)
		`
		if _, err := c.DB.Exec(querySubtasks, taskID, subtask); err != nil {
			return err
		}
	}

	return nil
}

func (c *Task) updateTaskInDB(email string, task *models.Task) error {
	const query = `
	UPDATE tasks
	SET title = $1,
	description = $2,
	due = $3
	WHERE id = $4;
	`
	due := time.Unix(task.Due, 0)
	if _, err := c.DB.Exec(query, task.Title, task.Description, due, task.ID); err != nil {
		return err
	}

	const querySubtasksDelete = `
	DELETE FROM subtasks
	WHERE task_id = $1
	`
	if _, err := c.DB.Exec(querySubtasksDelete, task.ID); err != nil {
		return err
	}

	for _, subtask := range task.Subtasks {
		const querySubtasks = `
		INSERT INTO subtasks(task_id, description)
		VALUES ($1, $2)
		`
		if _, err := c.DB.Exec(querySubtasks, task.ID, subtask); err != nil {
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
