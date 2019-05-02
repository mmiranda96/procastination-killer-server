package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"net/url"
	"os"

	"github.com/gorilla/mux"
	_ "github.com/lib/pq" // Postgres

	"github.com/mmiranda96/procastination-killer-server/controllers"
)

const (
	defaultPort = "8080"

	defaultPostgresHost     = "127.0.0.1"
	defaultPostgresPort     = "5432"
	defaultPostgresDatabase = "procastination-killer"
	defaultPostgresUser     = "postgres"
	defaultPostgresPassword = "postgres"

	defaultSMTPAddress = "smtp-relay.gmail.com:587"

	postgresURLEnvVar      = "DATABASE_URL"
	portEnvVar             = "PORT"
	postgresHostEnvVar     = "POSTGRES_HOST"
	postgresPortEnvVar     = "POSTGRES_PORT"
	postgresDatabaseEnvVar = "POSTGRES_DATABASE"
	postgresUserEnvVar     = "POSTGRES_USER"
	postgresPasswordEnvVar = "POSTGRES_PASSWORD"
	smtpAddressEnvVar      = "SMTP_ADDRESS"
	mailEmailEnvVar        = "MAIL_EMAIL"
	mailPasswordEnvVar     = "MAIL_PASSWORD"
)

var (
	port string

	postgresURL      string
	postgresHost     string
	postgresPort     string
	postgresDatabase string
	postgresUser     string
	postgresPassword string

	smtpAddress  string
	mailEmail    string
	mailPassword string
)

func init() {
	port = os.Getenv(portEnvVar)
	if port == "" {
		port = defaultPort

	}

	postgresURL = os.Getenv(postgresURLEnvVar)
	if postgresURL == "" {
		postgresHost = os.Getenv(postgresHostEnvVar)
		if postgresHost == "" {
			postgresHost = defaultPostgresHost
		}

		postgresPort = os.Getenv(postgresPortEnvVar)
		if postgresPort == "" {
			postgresPort = defaultPostgresPort
		}

		postgresDatabase = os.Getenv(postgresDatabaseEnvVar)
		if postgresDatabase == "" {
			postgresDatabase = defaultPostgresDatabase
		}

		postgresUser = os.Getenv(postgresUserEnvVar)
		if postgresUser == "" {
			postgresUser = defaultPostgresUser
		}

		postgresPassword = os.Getenv(postgresPasswordEnvVar)
		if postgresPassword == "" {
			postgresPassword = defaultPostgresPassword
		}
	}

	smtpAddress = os.Getenv(smtpAddressEnvVar)
	if smtpAddress == "" {
		smtpAddress = defaultSMTPAddress
	}

	mailEmail = os.Getenv(mailEmailEnvVar)
	mailPassword = os.Getenv(mailPasswordEnvVar)
}

func connect() (*sql.DB, error) {
	var connectionURL string
	if postgresURL != "" {
		connectionURL = postgresURL
	} else {
		ssl := url.Values{}
		ssl.Set("sslmode", "disable")

		dsn := url.URL{
			Scheme:   "postgres",
			User:     url.UserPassword(postgresUser, postgresPassword),
			Host:     fmt.Sprintf("%s:%s", postgresHost, postgresPort),
			Path:     postgresDatabase,
			RawQuery: ssl.Encode(),
		}
		connectionURL = dsn.String()
	}

	db, err := sql.Open("postgres", connectionURL)
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

	userController := &controllers.User{
		DB:             db,
		DeepLinkPrefix: "https://procastination-killer.com",
		SMTPAddress:    smtpAddress,
		Email:          mailEmail,
		Auth:           smtp.PlainAuth("", mailEmail, mailPassword, smtpAddress),
	}
	taskController := &controllers.Task{DB: db}

	server := mux.NewRouter()

	server.HandleFunc("/tasks", taskController.GetTasks).Methods(http.MethodGet)
	server.HandleFunc("/tasks", taskController.CreateTask).Methods(http.MethodPost)
	server.HandleFunc("/tasks", taskController.UpdateTask).Methods(http.MethodPut)
	server.HandleFunc("/tasks/{taskID}/addUser", taskController.AddUserToTask).Methods(http.MethodPost)

	server.HandleFunc("/users/login", userController.Login).Methods(http.MethodPost)
	server.HandleFunc("/users", userController.CreateUser).Methods(http.MethodPost)
	server.HandleFunc("/users", userController.UpdateUser).Methods(http.MethodPut)
	server.HandleFunc("/users/sendResetPasswordEmail", userController.SendPasswordResetEmail).Methods(http.MethodPost)
	server.HandleFunc("/users/resetPassword", userController.ResetPassword).Methods(http.MethodPost)

	mux := http.NewServeMux()
	authenticationMiddleware := userController.NewAuthenticationMiddleware()
	mux.Handle("/tasks", authenticationMiddleware(server))
	mux.Handle("/", server)

	address := fmt.Sprintf("0.0.0.0:%s", port)
	log.Printf("Listening on %s\n", address)
	log.Fatalln(http.ListenAndServe(address, mux))
}
