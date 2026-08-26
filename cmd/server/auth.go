package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const sessionLifetime = 7 * 24 * time.Hour

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (app *application) login(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var input loginRequest

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{
			Error: "invalid request",
		})
		return
	}

	input.Email = strings.ToLower(strings.TrimSpace(input.Email))

	if input.Email == "" || input.Password == "" {
		invalidLoginResponse(w)
		return
	}

	// Keep login password input bounded
	if len(input.Password) > 128 {
		invalidLoginResponse(w)
		return
	}

	var (
		userID       int64
		passwordHash string
		status       string
	)

	err := app.db.QueryRowContext(
		r.Context(),
		`
			SELECT
				user_id,
				password_hash,
				status
			FROM users
			WHERE LOWER(email) = $1
		`,
		input.Email,
	).Scan(
		&userID,
		&passwordHash,
		&status,
	)

	if errors.Is(err, sql.ErrNoRows) {
		if _, verifyErr := verifyPassword(
			input.Password,
			app.dummyPasswordHash,
		); verifyErr != nil {
			log.Printf("verify dummy password: %v", verifyErr)
		}

		invalidLoginResponse(w)
		return
	}

	if err != nil {
		log.Printf("find user during login: %v", err)

		writeJSON(w, http.StatusInternalServerError, errorResponse{
			Error: "internal server error",
		})
		return
	}

	passwordMatches, err := verifyPassword(
		input.Password,
		passwordHash,
	)
	if err != nil {
		log.Printf("verify password for user %d: %v", userID, err)

		writeJSON(w, http.StatusInternalServerError, errorResponse{
			Error: "internal server error",
		})
		return
	}

	if !passwordMatches {
		invalidLoginResponse(w)
		return
	}

	switch status {
	case "pending":
		writeJSON(w, http.StatusForbidden, errorResponse{
			Error: "email verification is required",
		})
		return

	case "disabled":
		writeJSON(w, http.StatusForbidden, errorResponse{
			Error: "account is unavailable",
		})
		return

	case "active":
		// Active

	default:
		log.Printf(
			"user %d has unexpected account status %q",
			userID,
			status,
		)

		writeJSON(w, http.StatusInternalServerError, errorResponse{
			Error: "internal server error",
		})
		return
	}

	rawToken, expiresAt, err := app.createSession(
		r.Context(),
		userID,
		r.UserAgent(),
	)
	if err != nil {
		log.Printf("create session for user %d: %v", userID, err)

		writeJSON(w, http.StatusInternalServerError, errorResponse{
			Error: "could not create session",
		})
		return
	}

	_, err = app.db.ExecContext(
		r.Context(),
		`
			UPDATE users
			SET last_login_at = NOW()
			WHERE user_id = $1
		`,
		userID,
	)
	if err != nil {
		log.Printf("update last login for user %d: %v", userID, err)
	}

	secureCookie := strings.HasPrefix(app.baseURL, "https://")

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    rawToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   secureCookie,
		SameSite: http.SameSiteLaxMode,
		Expires:  expiresAt,
		MaxAge:   int(sessionLifetime.Seconds()),
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"message": "login successful",
	})
}

func invalidLoginResponse(w http.ResponseWriter) {
	writeJSON(w, http.StatusUnauthorized, errorResponse{
		Error: "invalid email or password",
	})
}

func (app *application) createSession(
	ctx context.Context,
	userID int64,
	userAgent string,
) (string, time.Time, error) {
	// This is the secret that will go into the browser cookie
	rawToken, err := generateRandomToken(32)
	if err != nil {
		return "", time.Time{}, fmt.Errorf(
			"generate session token: %w",
			err,
		)
	}

	// This is a separate identifier for the session itself
	sessionID, err := generateRandomToken(16)
	if err != nil {
		return "", time.Time{}, fmt.Errorf(
			"generate session ID: %w",
			err,
		)
	}

	tokenHash := hashSessionToken(rawToken)

	now := time.Now().UTC()
	expiresAt := now.Add(sessionLifetime)

	sessionKey := "session:" + tokenHash
	userSessionsKey := fmt.Sprintf("user_sessions:%d", userID)

	// Don't allow arbitrarily large User-Agent metadata into Redis
	if len(userAgent) > 512 {
		userAgent = userAgent[:512]
	}

	pipe := app.redisClient.TxPipeline()

	pipe.HSet(ctx, sessionKey, map[string]any{
		"user_id":      userID,
		"session_id":   sessionID,
		"created_at":   now.Unix(),
		"last_seen_at": now.Unix(),
		"user_agent":   userAgent,
		"expires_at":   expiresAt.Unix(),
	})

	// Redis automatically removes the session key after this duration.
	pipe.Expire(ctx, sessionKey, sessionLifetime)

	pipe.ZRemRangeByScore(
		ctx,
		userSessionsKey,
		"-inf",
		fmt.Sprintf("%d", now.Unix()),
	)

	pipe.ZAdd(ctx, userSessionsKey, redis.Z{
		Score:  float64(expiresAt.Unix()),
		Member: tokenHash,
	})

	pipe.Expire(ctx, userSessionsKey, sessionLifetime)

	if _, err := pipe.Exec(ctx); err != nil {
		return "", time.Time{}, fmt.Errorf(
			"store session in Redis: %w",
			err,
		)
	}

	return rawToken, expiresAt, nil
}

func generateRandomToken(byteLength int) (string, error) {
	buffer := make([]byte, byteLength)

	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func hashSessionToken(rawToken string) string {
	hash := sha256.Sum256([]byte(rawToken))

	return hex.EncodeToString(hash[:])
}
