package controllers

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/smtp"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/mmiranda96/procastination-killer-server/models"
)

// User is a contrller for users
type User struct {
	DB             *sql.DB
	DeepLinkPrefix string
	SMTPAddress    string
	Email          string
	Auth           smtp.Auth
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

// UpdateUser updates an existing user
func (c *User) UpdateUser(w http.ResponseWriter, r *http.Request) {
	authUser, err := c.getValidatedUserFromHeader(r.Header)
	if err != nil {
		log.Println(err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	} else if authUser == nil {
		http.Error(w, "Invalid email and/or password", http.StatusForbidden)
		return
	}

	body, _ := ioutil.ReadAll(r.Body)
	user := &models.User{}
	if err := json.Unmarshal(body, user); err != nil {
		log.Println(err)
		http.Error(w, "Invalid body", http.StatusBadRequest)
	}

	if err := c.updateUserInDB(authUser.Email, user); err != nil {
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

	storedUser, err := c.getUserFromDB(user.Email)
	if err != nil {
		log.Println(err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	} else if storedUser == nil || !doPasswordsMatch(user.Password, storedUser.Password) {
		http.Error(w, "Invalid email and/or password", http.StatusForbidden)
		return
	}

	storedUser.Password = ""
	w.Header().Set("Content-Type", "application/json")
	bytes, _ := json.Marshal(storedUser)
	w.Write(bytes)
}

type ctxKey string

const (
	userCtxKey = ctxKey("user")
)

// SendPasswordResetEmail sends a password reset email if the user exists
func (c *User) SendPasswordResetEmail(w http.ResponseWriter, r *http.Request) {
	body, _ := ioutil.ReadAll(r.Body)
	user := &models.User{}
	if err := json.Unmarshal(body, user); err != nil {
		log.Println(err)
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	storedUser, err := c.getUserFromDB(user.Email)
	if err != nil {
		log.Println(err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	} else if storedUser != nil {
		c.generateTokenAndSendEmail(storedUser.Email)
	}
}

// ResetPassword resets a password with a token
func (c *User) ResetPassword(w http.ResponseWriter, r *http.Request) {

}

// NewAuthenticationMiddleware creates a new authentication middleware
func (c *User) NewAuthenticationMiddleware() func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, err := c.getValidatedUserFromHeader(r.Header)
			if err != nil {
				log.Println(err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			} else if user == nil {
				http.Error(w, "Invalid email and/or password", http.StatusForbidden)
				return
			}

			ctx := context.WithValue(r.Context(), userCtxKey, user) // Ignore this warning
			h.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

const (
	authenticationHeader             = "Authorization"
	authenticationHeaderPrefix       = "Basic "
	authenticationHeaderPrefixLength = len(authenticationHeaderPrefix)
)

func (c *User) getValidatedUserFromHeader(header http.Header) (*models.User, error) {
	authHeader := header.Get(authenticationHeader)
	if len(authHeader) <= authenticationHeaderPrefixLength || authHeader[:authenticationHeaderPrefixLength] != authenticationHeaderPrefix {
		return nil, nil
	}

	data, err := base64.StdEncoding.DecodeString(authHeader[authenticationHeaderPrefixLength:])
	if err != nil {
		return nil, nil
	}
	values := strings.Split(string(data), ":")
	if len(values) != 2 {
		return nil, nil
	}

	user := &models.User{
		Email:    values[0],
		Password: values[1],
	}

	storedUser, err := c.getUserFromDB(user.Email)
	if err != nil {
		return nil, err
	} else if storedUser == nil || !doPasswordsMatch(user.Password, storedUser.Password) {
		return nil, nil
	}

	return user, nil
}

func (c *User) getUserFromDB(email string) (*models.User, error) {
	const query = `
	SELECT email, name, password
	FROM users
	WHERE email = $1;
	`
	user := &models.User{}
	switch err := c.DB.QueryRow(query, email).Scan(&user.Email, &user.Name, &user.Password); err {
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
	INSERT INTO users(email, name, password)
	VALUES ($1, $2, $3);
	`
	_, err := c.DB.Exec(query, user.Email, user.Name, hashPassword(user.Password))

	return err
}

func (c *User) updateUserInDB(currentEmail string, user *models.User) error {
	const query = `
	UPDATE users
	SET name = $1, email = $2
	WHERE email = $3;
	`
	_, err := c.DB.Exec(query, user.Name, user.Email, currentEmail)

	return err
}

func (c *User) generateTokenAndSendEmail(email string) {
	go func() {
		token := uuid.New().String()
		if err := c.sendPasswordResetEmail(email, token); err != nil {
			log.Println(err)
		} else {
			log.Println("Email sent successfully.")
		}
	}()
}

func (c *User) sendPasswordResetEmail(email, token string) error {
	data := []byte(fmt.Sprintf("Click here to restore your email: %s%s", c.DeepLinkPrefix, token))
	return smtp.SendMail(c.SMTPAddress, nil, c.Email, []string{email}, data)
}

func hashPassword(password string) string {
	hashed, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)

	return string(hashed)
}

func doPasswordsMatch(given, expected string) bool {
	return bcrypt.CompareHashAndPassword([]byte(expected), []byte(given)) == nil
}
