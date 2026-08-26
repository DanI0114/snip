package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"
)

var errUnauthenticated = errors.New("unauthenticated")

type contextKey string

const authenticatedUserIDKey contextKey = "authenticatedUserID"

func (app *application) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, err := app.authenticateRequest(r)

		if errors.Is(err, errUnauthenticated) {
			writeJSON(w, http.StatusUnauthorized, errorResponse{
				Error: "authentication required",
			})
			return
		}

		if err != nil {
			log.Printf("authenticate request: %v", err)

			writeJSON(w, http.StatusInternalServerError, errorResponse{
				Error: "internal server error",
			})
			return
		}

		ctx := context.WithValue(
			r.Context(),
			authenticatedUserIDKey,
			userID,
		)

		next.ServeHTTP(
			w,
			r.WithContext(ctx),
		)
	})
}

func (app *application) authenticateRequest(r *http.Request) (int64, error) {
	cookie, err := r.Cookie("session")

	if errors.Is(err, http.ErrNoCookie) {
		return 0, errUnauthenticated
	}

	if err != nil {
		return 0, fmt.Errorf("read session cookie: %w", err)
	}

	if !validSessionToken(cookie.Value) {
		return 0, errUnauthenticated
	}

	tokenHash := hashSessionToken(cookie.Value)

	sessionKey := "session:" + tokenHash

	values, err := app.redisClient.HMGet(
		r.Context(),
		sessionKey,
		"user_id",
		"expires_at",
	).Result()

	if err != nil {
		return 0, fmt.Errorf("read session from Redis: %w", err)
	}

	// HMGET gives us nil values when the session doesn't exist.
	if len(values) != 2 ||
		values[0] == nil ||
		values[1] == nil {
		return 0, errUnauthenticated
	}

	userID, err := strconv.ParseInt(
		fmt.Sprint(values[0]),
		10,
		64,
	)
	if err != nil {
		return 0, fmt.Errorf("invalid session user ID: %w", err)
	}

	expiresAtUnix, err := strconv.ParseInt(
		fmt.Sprint(values[1]),
		10,
		64,
	)
	if err != nil {
		return 0, fmt.Errorf("invalid session expiration: %w", err)
	}

	if time.Now().UTC().Unix() >= expiresAtUnix {
		_ = app.deleteSession(
			r.Context(),
			userID,
			tokenHash,
		)

		return 0, errUnauthenticated
	}

	// Redis tells us that this is a valid session,
	// but PostgreSQL remains authoritative about the account itself.
	var status string

	err = app.db.QueryRowContext(
		r.Context(),
		`
			SELECT status
			FROM users
			WHERE user_id = $1
		`,
		userID,
	).Scan(&status)

	if errors.Is(err, sql.ErrNoRows) {
		_ = app.deleteSession(
			r.Context(),
			userID,
			tokenHash,
		)

		return 0, errUnauthenticated
	}

	if err != nil {
		return 0, fmt.Errorf(
			"check authenticated user: %w",
			err,
		)
	}

	if status != "active" {
		_ = app.deleteSession(
			r.Context(),
			userID,
			tokenHash,
		)

		return 0, errUnauthenticated
	}

	return userID, nil
}

func validSessionToken(token string) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return false
	}

	return len(decoded) == 32
}

func (app *application) deleteSession(
	ctx context.Context,
	userID int64,
	tokenHash string,
) error {
	sessionKey := "session:" + tokenHash
	userSessionsKey := fmt.Sprintf(
		"user_sessions:%d",
		userID,
	)

	pipe := app.redisClient.TxPipeline()

	pipe.Del(ctx, sessionKey)

	pipe.ZRem(
		ctx,
		userSessionsKey,
		tokenHash,
	)

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}

	return nil
}

func currentUserID(r *http.Request) (int64, bool) {
	userID, ok := r.Context().
		Value(authenticatedUserIDKey).(int64)

	return userID, ok
}

func (app *application) me(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(r)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, errorResponse{
			Error: "authenticated user missing",
		})
		return
	}

	var (
		name  string
		email string
		role  string
	)

	err := app.db.QueryRowContext(
		r.Context(),
		`
			SELECT name, email, role
			FROM users
			WHERE user_id = $1
		`,
		userID,
	).Scan(
		&name,
		&email,
		&role,
	)

	if err != nil {
		log.Printf("load current user %d: %v", userID, err)

		writeJSON(w, http.StatusInternalServerError, errorResponse{
			Error: "internal server error",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"user_id": userID,
		"name":    name,
		"email":   email,
		"role":    role,
	})
}
