package controllers

import (
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/mail"
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
	Password       string
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
	body, _ := ioutil.ReadAll(r.Body)
	request := &models.ResetPasswordRequest{}
	if err := json.Unmarshal(body, request); err != nil {
		log.Println(err)
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	if ok, err := c.changePassword(request.Token, request.Email, request.Password); err != nil {
		log.Println(err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	} else if !ok {
		http.Error(w, "Invalid request", http.StatusUnauthorized)
		return
	}
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

func (c *User) storeResetPasswordToken(email, token string) error {
	const query = `
	INSERT INTO reset_password_tokens(id, email)
	VALUES ($1, $2);
	`
	_, err := c.DB.Exec(query, token, email)

	return err
}

func (c *User) changePassword(token, email, password string) (bool, error) {
	tx, err := c.DB.Begin()
	if err != nil {
		return false, err
	}

	const tokenQuery = `
	SELECT email
	FROM reset_password_tokens
	WHERE id = $1;
	`
	var tokenEmail string
	if err := tx.QueryRow(tokenQuery, token).Scan(&tokenEmail); err != nil {
		tx.Rollback()
		if err == sql.ErrNoRows {
			return false, nil
		}

		return false, err
	}

	if email != tokenEmail {
		tx.Rollback()
		return false, nil
	}

	const updateQuery = `
	UPDATE users
	SET password = $1
	WHERE email = $2;
	`
	if _, err := tx.Exec(updateQuery, hashPassword(password), email); err != nil {
		tx.Rollback()
		return false, err
	}

	const deleteQuery = `
	DELETE FROM reset_password_tokens
	WHERE id = $1;`
	if _, err := tx.Exec(deleteQuery, token); err != nil {
		tx.Rollback()
		return false, err
	}

	if err := tx.Commit(); err != nil {
		return false, err
	}

	return true, nil
}

func (c *User) generateTokenAndSendEmail(email string) {
	go func() {
		token := uuid.New().String()
		if err := c.storeResetPasswordToken(email, token); err != nil {
			log.Println(err)
			return
		}

		if err := c.sendPasswordResetEmail(email, token); err != nil {
			log.Println(err)
			return
		}
		log.Println("Email sent successfully")
	}()
}

const (
	emailFromName = "Procastination Killer"
	emailSubject  = "Password reset"
)

func (c *User) sendPasswordResetEmail(email, token string) error {
	data := fmt.Sprintf("Click here to restore your email: %s%s", c.DeepLinkPrefix, token)

	// Code taken from https://gist.github.com/chrisgillis/10888032
	from := mail.Address{Name: "Procastination Killer", Address: c.Email}
	to := mail.Address{Name: "", Address: email}

	// Setup headers
	headers := make(map[string]string)
	headers["From"] = from.String()
	headers["To"] = to.String()
	headers["Subject"] = emailSubject

	// Setup message
	message := ""
	for k, v := range headers {
		message += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	message += "\r\n" + data

	host, _, _ := net.SplitHostPort(c.SMTPAddress)
	auth := smtp.PlainAuth("", c.Email, c.Password, host)

	// TLS config
	tlsconfig := &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         host,
	}

	// Here is the key, you need to call tls.Dial instead of smtp.Dial
	// for smtp servers running on 465 that require an ssl connection
	// from the very beginning (no starttls)
	conn, err := tls.Dial("tcp", c.SMTPAddress, tlsconfig)
	if err != nil {
		return err
	}

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}

	// Auth
	if err = client.Auth(auth); err != nil {
		return err
	}

	// To && From
	if err = client.Mail(from.Address); err != nil {
		return err
	}

	if err = client.Rcpt(to.Address); err != nil {
		return err
	}

	// Data
	w, err := client.Data()
	if err != nil {
		return err
	}

	_, err = w.Write([]byte(message))
	if err != nil {
		return err
	}

	err = w.Close()
	if err != nil {
		return err
	}

	return client.Quit()
}

func hashPassword(password string) string {
	hashed, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)

	return string(hashed)
}

func doPasswordsMatch(given, expected string) bool {
	return bcrypt.CompareHashAndPassword([]byte(expected), []byte(given)) == nil
}
