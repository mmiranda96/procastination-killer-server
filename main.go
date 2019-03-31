package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"net/url"

	"github.com/gorilla/mux"
	_ "github.com/lib/pq" // Postgres

	"github.com/mmiranda96/procastination-killer-server/controllers"
)

const dbFile = "db.json"

func connect() (*sql.DB, error) {
	ssl := url.Values{}
	ssl.Set("sslmode", "disable")

	dsn := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword("postgres", "postgres"),
		Host:     fmt.Sprintf("127.0.0.1:5432"),
		Path:     "procastination-killer",
		RawQuery: ssl.Encode(),
	}

	db, err := sql.Open("postgres", dsn.String())
	if err != nil {
		return nil, fmt.Errorf("unable to create postgres database driver: %v", err)

	}

	if err = db.Ping(); err != nil {
		return nil, fmt.Errorf("unable to connect: %v", err)
	}

	return db, nil
}

func main() {
	db, err := connect()
	if err != nil {
		log.Fatalln(err)
	}

	userController := &controllers.User{DB: db}
	taskController := &controllers.Task{DB: db}

	server := mux.NewRouter()

	server.HandleFunc("/tasks", taskController.GetTasks).Methods(http.MethodGet)
	server.HandleFunc("/tasks", taskController.CreateTask).Methods(http.MethodPost)
	server.HandleFunc("/tasks", taskController.UpdateTask).Methods(http.MethodPut)

	server.HandleFunc("/users/login", userController.Login).Methods(http.MethodPost)
	server.HandleFunc("/users", userController.CreateUser).Methods(http.MethodPost)

	mux := http.NewServeMux()
	authenticationMiddleware := userController.NewAuthenticationMiddleware()
	mux.Handle("/tasks", authenticationMiddleware(server))
	mux.Handle("/", server)

	address := ":8080"
	log.Printf("Listening on %s\n", address)
	log.Fatalln(http.ListenAndServe(address, mux))
}
