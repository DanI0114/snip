package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const verificationLifetime = 30 * time.Minute

type resendVerificationRequest struct {
	Email string `json:"email"`
}

func (app *application) issueEmailVerification(
	ctx context.Context,
	userID int64,
	name string,
	email string,
) error {

	rawToken, err := generateRandomToken(32)
	if err != nil {
		return fmt.Errorf(
			"generate verification token: %w",
			err,
		)
	}

	tokenHash := hashSessionToken(rawToken)

	tokenKey := "email_verify:" + tokenHash

	userTokenKey := fmt.Sprintf(
		"email_verify_user:%d",
		userID,
	)

	oldHash, err := app.redisClient.Get(
		ctx,
		userTokenKey,
	).Result()

	if err != nil && !errors.Is(err, redis.Nil) {
		return fmt.Errorf(
			"read existing verification token: %w",
			err,
		)
	}

	pipe := app.redisClient.TxPipeline()

	if oldHash != "" {
		pipe.Del(
			ctx,
			"email_verify:"+oldHash,
		)
	}
	pipe.Set(
		ctx,
		tokenKey,
		strconv.FormatInt(userID, 10),
		verificationLifetime,
	)
	pipe.Set(
		ctx,
		userTokenKey,
		tokenHash,
		verificationLifetime,
	)

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf(
			"store verification token: %w",
			err,
		)
	}

	verificationURL :=
		app.baseURL +
			"/verify-email?token=" +
			url.QueryEscape(rawToken)

	if err := app.sendVerificationEmail(
		ctx,
		name,
		email,
		verificationURL,
	); err != nil {
		cleanup := app.redisClient.TxPipeline()

		cleanup.Del(ctx, tokenKey)
		cleanup.Del(ctx, userTokenKey)

		if _, cleanupErr := cleanup.Exec(ctx); cleanupErr != nil {
			log.Printf(
				"cleanup verification token: %v",
				cleanupErr,
			)
		}

		return fmt.Errorf(
			"send verification email: %w",
			err,
		)
	}

	return nil
}

func (app *application) sendVerificationEmail(
	ctx context.Context,
	name string,
	email string,
	verificationURL string,
) error {
	if app.emailMode == "log" {
		log.Printf(
			"EMAIL VERIFICATION for %s <%s>: %s",
			name,
			email,
			verificationURL,
		)

		return nil
	}
	if app.emailMode != "brevo" {
		return fmt.Errorf(
			"unsupported email mode: %s",
			app.emailMode,
		)
	}
	if app.brevoAPIKey == "" {
		return errors.New("Brevo API key is not configured")
	}
	if app.emailFrom == "" {
		return errors.New("email sender is not configured")
	}

	safeName := html.EscapeString(name)
	safeURL := html.EscapeString(verificationURL)

	htmlContent := fmt.Sprintf(`
		<div style="
			font-family: Arial, sans-serif;
			max-width: 560px;
			margin: 40px auto;
			color: #12141C;
		">

			<h2>
				Verify your Snip account
			</h2>

			<p>
				Hello %s,
			</p>

			<p>
				Thanks for creating a Snip account.
				Click the button below to verify your email address.
			</p>

			<p style="margin: 30px 0;">
				<a
					href="%s"
					style="
						display: inline-block;
						padding: 12px 18px;
						background: #E8B94A;
						color: #12141C;
						text-decoration: none;
						border-radius: 4px;
						font-weight: 600;
					"
				>
					Verify email
				</a>
			</p>

			<p>
				This verification link expires in 30 minutes.
			</p>

			<p style="color: #666;">
				If you didn't create this Snip account,
				you can ignore this email.
			</p>

		</div>
	`,
		safeName,
		safeURL,
	)
	payload := map[string]any{
		"sender": map[string]string{
			"name":  app.emailFromName,
			"email": app.emailFrom,
		},

		"to": []map[string]string{
			{
				"name":  name,
				"email": email,
			},
		},
		"subject":     "Verify your Snip account",
		"htmlContent": htmlContent,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf(
			"encode Brevo email request: %w",
			err,
		)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		"https://api.brevo.com/v3/smtp/email",
		bytes.NewReader(body),
	)

	if err != nil {
		return fmt.Errorf(
			"create Brevo request: %w",
			err,
		)
	}
	req.Header.Set(
		"Content-Type",
		"application/json",
	)
	req.Header.Set(
		"Accept",
		"application/json",
	)
	req.Header.Set(
		"api-key",
		app.brevoAPIKey,
	)
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	response, err := client.Do(req)
	if err != nil {
		return fmt.Errorf(
			"send Brevo request: %w",
			err,
		)
	}

	defer response.Body.Close()
	if response.StatusCode < 200 ||
		response.StatusCode >= 300 {

		responseBody, _ := io.ReadAll(
			io.LimitReader(
				response.Body,
				4096,
			),
		)

		return fmt.Errorf(
			"Brevo returned status %d: %s",
			response.StatusCode,
			strings.TrimSpace(
				string(responseBody),
			),
		)
	}
	return nil
}

func (app *application) verifyEmail(
	w http.ResponseWriter,
	r *http.Request,
) {

	rawToken := r.URL.Query().Get("token")

	if !validVerificationToken(rawToken) {
		http.Redirect(
			w,
			r,
			"/login?verification=invalid",
			http.StatusSeeOther,
		)

		return
	}

	tokenHash := hashSessionToken(rawToken)

	tokenKey := "email_verify:" + tokenHash

	userIDString, err := app.redisClient.Get(
		r.Context(),
		tokenKey,
	).Result()

	if errors.Is(err, redis.Nil) {
		http.Redirect(
			w,
			r,
			"/login?verification=invalid",
			http.StatusSeeOther,
		)

		return
	}

	if err != nil {
		log.Printf(
			"read verification token: %v",
			err,
		)

		http.Error(
			w,
			"internal server error",
			http.StatusInternalServerError,
		)

		return
	}

	userID, err := strconv.ParseInt(
		userIDString,
		10,
		64,
	)

	if err != nil {
		log.Printf(
			"invalid verification user ID: %v",
			err,
		)

		http.Error(
			w,
			"internal server error",
			http.StatusInternalServerError,
		)

		return
	}

	result, err := app.db.ExecContext(
		r.Context(),
		`
			UPDATE users
			SET
				status = 'active',
				email_verified_at = NOW()
			WHERE
				user_id = $1
				AND status = 'pending'
		`,
		userID,
	)

	if err != nil {
		log.Printf(
			"verify user %d: %v",
			userID,
			err,
		)

		http.Error(
			w,
			"internal server error",
			http.StatusInternalServerError,
		)

		return
	}

	rowsAffected, err := result.RowsAffected()

	if err != nil {
		log.Printf(
			"read verification result: %v",
			err,
		)

		http.Error(
			w,
			"internal server error",
			http.StatusInternalServerError,
		)

		return
	}

	if rowsAffected != 1 {
		http.Redirect(
			w,
			r,
			"/login?verification=invalid",
			http.StatusSeeOther,
		)

		return
	}

	userTokenKey := fmt.Sprintf(
		"email_verify_user:%d",
		userID,
	)

	pipe := app.redisClient.TxPipeline()

	pipe.Del(
		r.Context(),
		tokenKey,
	)

	pipe.Del(
		r.Context(),
		userTokenKey,
	)

	if _, err := pipe.Exec(r.Context()); err != nil {
		log.Printf(
			"delete used verification token: %v",
			err,
		)
	}

	http.Redirect(
		w,
		r,
		"/login?verified=1",
		http.StatusSeeOther,
	)
}

func (app *application) resendVerification(
	w http.ResponseWriter,
	r *http.Request,
) {

	r.Body = http.MaxBytesReader(
		w,
		r.Body,
		1<<20,
	)

	var input resendVerificationRequest

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&input); err != nil {
		writeJSON(
			w,
			http.StatusBadRequest,
			errorResponse{
				Error: "invalid request",
			},
		)

		return
	}

	input.Email = strings.ToLower(
		strings.TrimSpace(input.Email),
	)

	// Always use a generic public response.
	genericResponse := func() {
		writeJSON(
			w,
			http.StatusAccepted,
			map[string]any{
				"message": "if the account requires verification, a new verification email will be sent",
			},
		)
	}

	if validateEmail(input.Email) != nil {
		genericResponse()
		return
	}

	rateKey :=
		"verify_resend:" +
			hashSessionToken(input.Email)

	allowed, err := app.redisClient.SetNX(
		r.Context(),
		rateKey,
		"1",
		time.Minute,
	).Result()

	if err != nil {
		log.Printf(
			"verification resend rate limit: %v",
			err,
		)

		writeJSON(
			w,
			http.StatusInternalServerError,
			errorResponse{
				Error: "internal server error",
			},
		)

		return
	}

	if !allowed {
		genericResponse()
		return
	}

	var (
		userID int64
		name   string
		status string
	)

	err = app.db.QueryRowContext(
		r.Context(),
		`
			SELECT
				user_id,
				name,
				status
			FROM users
			WHERE LOWER(email) = $1
		`,
		input.Email,
	).Scan(
		&userID,
		&name,
		&status,
	)

	if errors.Is(err, sql.ErrNoRows) {
		genericResponse()
		return
	}

	if err != nil {
		log.Printf(
			"find user for verification resend: %v",
			err,
		)

		writeJSON(
			w,
			http.StatusInternalServerError,
			errorResponse{
				Error: "internal server error",
			},
		)

		return
	}

	if status != "pending" {
		genericResponse()
		return
	}

	if err := app.issueEmailVerification(
		r.Context(),
		userID,
		name,
		input.Email,
	); err != nil {

		log.Printf(
			"resend verification for user %d: %v",
			userID,
			err,
		)

		genericResponse()
		return
	}

	genericResponse()
}

func validVerificationToken(token string) bool {

	decoded, err :=
		base64.RawURLEncoding.DecodeString(token)

	if err != nil {
		return false
	}

	return len(decoded) == 32
}
