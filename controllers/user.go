package controllers

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"strings"

	"github.com/mmiranda96/procastination-killer-server/models"
)

type ctxKey string

const (
	authenticationHeader             = "Authorization"
	authenticationHeaderPrefix       = "Basic "
	authenticationHeaderPrefixLength = len(authenticationHeaderPrefix)
	userCtxKey                       = ctxKey("user")
)

func getUserFromHeader(header string) (*models.User, error) {
	if len(header) <= authenticationHeaderPrefixLength || header[:authenticationHeaderPrefixLength] != authenticationHeaderPrefix {
		return nil, errors.New("Bad authentication")
	}
	data, err := base64.StdEncoding.DecodeString(header[authenticationHeaderPrefixLength:])
	if err != nil {
		return nil, err
	}
	values := strings.Split(string(data), ":")
	if len(values) != 2 {
		return nil, errors.New("Bad authentication")
	}

	return &models.User{
		Email:    values[0],
		Password: values[1],
	}, nil
}

// User is a contrller for users
type User struct {
	DB *sql.DB
}

// CreateUser creates a new user
func (c *User) CreateUser(w http.ResponseWriter, r *http.Request) {
	body, _ := ioutil.ReadAll(r.Body)
	user := &models.User{}
	if err := json.Unmarshal(body, user); err != nil {
		log.Println(err)
		http.Error(w, "Invalid body", http.StatusBadRequest)
	}

	if err := c.createUserInDB(user); err != nil {
		log.Println(err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// Login validates an email with a password
func (c *User) Login(w http.ResponseWriter, r *http.Request) {
	body, _ := ioutil.ReadAll(r.Body)
	user := &models.User{}
	if err := json.Unmarshal(body, user); err != nil {
		log.Println(err)
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	if storedUser, err := c.getUserFromDB(user.Email); err != nil {
		log.Println(err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	} else if storedUser == nil || storedUser.Password != user.Password {
		http.Error(w, "Invalid email and/or password", http.StatusForbidden)
		return
	}
}

// NewAuthenticationMiddleware creates a new authentication middleware
func (c *User) NewAuthenticationMiddleware() func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, err := getUserFromHeader(r.Header.Get(authenticationHeader))
			if err != nil {
				log.Println(err)
				http.Error(w, err.Error(), http.StatusForbidden)
				return
			}

			if storedUser, err := c.getUserFromDB(user.Email); err != nil {
				log.Println(err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			} else if storedUser == nil || storedUser.Password != user.Password {
				http.Error(w, "Invalid email and/or password", http.StatusForbidden)
				return
			}

			ctx := context.WithValue(r.Context(), userCtxKey, user) // Ignore this warning
			h.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func (c *User) getUserFromDB(email string) (*models.User, error) {
	const query = `
	SELECT email, password
	FROM users
	WHERE email = $1;
	`
	user := &models.User{}
	switch err := c.DB.QueryRow(query, email).Scan(&user.Email, &user.Password); err {
	case nil:
		return user, nil
	case sql.ErrNoRows:
		return nil, nil
	default:
		return nil, err
	}
}

func (c *User) createUserInDB(user *models.User) error {
	const query = `
	INSERT INTO users(email, password)
	VALUES ($1, $2);
	`
	_, err := c.DB.Exec(query, user.Email, user.Password)

	return err
}
