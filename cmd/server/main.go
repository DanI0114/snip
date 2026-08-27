package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

const (
	defaultAddress = ":8080"
	codeLength     = 7
	codeAlphabet   = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
)

type application struct {
	db                *sql.DB
	redisClient       *redis.Client
	baseURL           string
	dummyPasswordHash string

	emailMode     string
	brevoAPIKey   string
	emailFrom     string
	emailFromName string
}

type createLinkRequest struct {
	URL string `json:"url"`
}

type createLinkResponse struct {
	Code     string `json:"code"`
	ShortURL string `json:"short_url"`
	LongURL  string `json:"long_url"`
}

type errorResponse struct {
	Error string `json:"error"`
}

type registerRequest struct {
	Name                 string `json:"name"`
	Email                string `json:"email"`
	Password             string `json:"password"`
	PasswordConfirmation string `json:"password_confirmation"`
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file found")
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}

	baseURL := strings.TrimRight(os.Getenv("BASE_URL"), "/")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		log.Fatal("REDIS_URL variable is required")
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	redisOptions, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Fatalf("parse Redis URL: %v", err)
	}

	redisClient := redis.NewClient(redisOptions)
	defer redisClient.Close()

	// explicitly verify the database at startup
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("connect to database: %v", err)
	}

	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("connect to redis: %v", err)
	}

	// Keeping the pool small for the MVP
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxIdleTime(5 * time.Minute)

	dummyPasswordHash, err := hashPassword(
		"this-password-does-not-belong-to-any-user",
	)

	emailMode := envOrDefault("EMAIL_MODE", "log")

	brevoAPIKey := os.Getenv("BREVO_API_KEY")
	emailFrom := os.Getenv("EMAIL_FROM")
	emailFromName := envOrDefault("EMAIL_FROM_NAME", "Snip")

	switch emailMode {
	case "log":
		log.Println("email mode: log")

	case "brevo":
		if brevoAPIKey == "" {
			log.Fatal("BREVO_API_KEY is required when EMAIL_MODE=brevo")
		}

		if emailFrom == "" {
			log.Fatal("EMAIL_FROM is required when EMAIL_MODE=brevo")
		}

		log.Println("email mode: brevo")

	default:
		log.Fatalf("unsupported EMAIL_MODE: %s", emailMode)
	}

	app := &application{
		db:                db,
		redisClient:       redisClient,
		baseURL:           baseURL,
		dummyPasswordHash: dummyPasswordHash,

		emailMode:     emailMode,
		brevoAPIKey:   brevoAPIKey,
		emailFrom:     emailFrom,
		emailFromName: emailFromName,
	}

	// router
	mux := http.NewServeMux()

	// API
	mux.Handle("POST /api/links", app.requireAuth(http.HandlerFunc(app.createLink)))
	mux.Handle("GET /api/links/mine", app.requireAuth(http.HandlerFunc(app.myLinks)))
	mux.Handle("GET /api/auth/me", app.requireAuth(http.HandlerFunc(app.me)))
	mux.Handle("POST /api/auth/logout", app.requireAuth(http.HandlerFunc(app.logout)))
	mux.Handle("DELETE /api/links/{code}", app.requireAuth(http.HandlerFunc(app.deleteLink)))

	// frontend
	staticFiles := http.FileServer(
		http.Dir("./webfiles/static"),
	)
	mux.Handle(
		"/static/",
		http.StripPrefix(
			"/static/",
			staticFiles,
		),
	)

	mux.HandleFunc("GET /{$}", app.home)
	mux.HandleFunc("POST /api/auth/register", app.register)
	mux.HandleFunc("POST /api/auth/login", app.login)
	mux.HandleFunc("GET /{code}", app.redirect)
	mux.HandleFunc("POST /api/auth/resend-verification", app.resendVerification)
	mux.HandleFunc("GET /verify-email", app.verifyEmail)
	mux.HandleFunc("GET /register", app.registerPage)
	mux.HandleFunc("GET /login", app.loginPage)
	mux.HandleFunc("GET /my-links", app.myLinksPage)

	server := &http.Server{
		Addr:              serverAddress(),
		Handler:           loggingMiddleware(mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Printf("server listening on %s", server.Addr)

	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func (app *application) registerPage(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "./webfiles/register.html")
}

func (app *application) loginPage(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "./webfiles/login.html")
}

func (app *application) home(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "./webfiles/index.html")
}

func validateEmail(email string) error {
	if email == "" {
		return errors.New("email is required")
	}

	if len(email) > 254 {
		return errors.New("email is too long")
	}

	address, err := mail.ParseAddress(email)
	if err != nil {
		return errors.New("invalid email")
	}

	if address.Address != email {
		return errors.New("invalid email")
	}

	return nil
}

func (app *application) register(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var input registerRequest

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{
			Error: "invalid request body",
		})
		return
	}

	input.Name = strings.TrimSpace(input.Name)
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))

	// name validation
	if input.Name == "" {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{
			Error: "name is required",
		})
		return
	}

	if len(input.Name) > 64 {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{
			Error: "name is too long",
		})
		return
	}

	// email validation
	if err := validateEmail(input.Email); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{
			Error: "invalid email address",
		})
		return
	}

	// password validation
	if len(input.Password) < 15 {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{
			Error: "password must be at least 15 characters",
		})
		return
	}

	if len(input.Password) > 128 {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{
			Error: "password is too long",
		})
		return
	}

	if input.Password != input.PasswordConfirmation {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{
			Error: "passwords do not match",
		})
		return
	}

	passwordHash, err := hashPassword(input.Password)
	if err != nil {
		log.Printf("hash password: %v", err)

		writeJSON(w, http.StatusInternalServerError, errorResponse{
			Error: "internal server error",
		})
		return
	}

	var userID int64

	err = app.db.QueryRowContext(
		r.Context(),
		`
		INSERT INTO users (
			name,
			email,
			password_hash
		)
		VALUES ($1, $2, $3)
		RETURNING user_id
	`,
		input.Name,
		input.Email,
		passwordHash,
	).Scan(&userID)

	if err != nil {
		var pgErr *pgconn.PgError

		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			writeJSON(
				w,
				http.StatusConflict,
				errorResponse{
					Error: "an account with that email already exists",
				},
			)
			return
		}

		log.Printf("insert user: %v", err)

		writeJSON(
			w,
			http.StatusInternalServerError,
			errorResponse{
				Error: "could not create account",
			},
		)
		return
	}

	// Only issue a verification token after we know
	// the user was successfully inserted.
	if err := app.issueEmailVerification(
		r.Context(),
		userID,
		input.Name,
		input.Email,
	); err != nil {
		log.Printf(
			"issue verification for user %d: %v",
			userID,
			err,
		)

		writeJSON(
			w,
			http.StatusServiceUnavailable,
			errorResponse{
				Error: "account created, but verification email could not be sent; please request another verification email",
			},
		)
		return
	}

	writeJSON(
		w,
		http.StatusAccepted,
		map[string]any{
			"message": "account created; check your email to verify your account",
		},
	)
}

func (app *application) logout(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(r)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, errorResponse{
			Error: "authenticated user missing",
		})
		return
	}

	cookie, err := r.Cookie("session")
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{
			Error: "authentication required",
		})
		return
	}

	tokenHash := hashSessionToken(cookie.Value)

	if err := app.deleteSession(
		r.Context(),
		userID,
		tokenHash,
	); err != nil {
		log.Printf("delete session for user %d: %v", userID, err)

		writeJSON(w, http.StatusInternalServerError, errorResponse{
			Error: "could not log out",
		})
		return
	}

	secureCookie := strings.HasPrefix(app.baseURL, "https://")

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secureCookie,
		SameSite: http.SameSiteLaxMode,

		MaxAge:  -1,
		Expires: time.Unix(1, 0),
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"message": "logged out",
	})
}

func (app *application) createLink(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var input createLinkRequest

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{
			Error: "request body must contain a valid url field",
		})
		return
	}

	targetURL, err := normalizeURL(input.URL)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{
			Error: err.Error(),
		})
		return
	}

	userID, ok := currentUserID(r)
	if !ok {
		writeJSON(
			w,
			http.StatusInternalServerError,
			errorResponse{
				Error: "authenticated user missing",
			},
		)
		return
	}

	for i := 0; i < 5; i++ {
		code, err := generateCode(7)
		if err != nil {
			log.Printf("generate short code: %v", err)

			writeJSON(
				w,
				http.StatusInternalServerError,
				errorResponse{
					Error: "could not generate short link",
				},
			)
			return
		}

		var savedCode string

		err = app.db.QueryRowContext(
			r.Context(),
			`
			INSERT INTO short_links (
				code,
				target_url,
				user_id
			)
			VALUES ($1, $2, $3)
			ON CONFLICT (code) DO NOTHING
			RETURNING code
		`,
			code,
			targetURL,
			userID,
		).Scan(&savedCode)

		if errors.Is(err, sql.ErrNoRows) {
			continue
		}

		if err != nil {
			log.Printf("save short link: %v", err)
			writeJSON(
				w,
				http.StatusInternalServerError,
				errorResponse{
					Error: "could not save short link",
				},
			)
			return
		}

		shortURL := app.baseURL + "/" + savedCode
		writeJSON(
			w,
			http.StatusCreated,
			map[string]any{
				"code":      savedCode,
				"short_url": shortURL,
			},
		)
		return
	}

	writeJSON(w, http.StatusInternalServerError, errorResponse{
		Error: "could not generate a unique short link",
	})
}

func (app *application) deleteLink(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(r)
	if !ok {
		writeJSON(
			w,
			http.StatusInternalServerError,
			errorResponse{
				Error: "authenticated user missing",
			},
		)
		return
	}

	code := r.PathValue("code")

	if !validCode(code) {
		writeJSON(
			w,
			http.StatusBadRequest,
			errorResponse{
				Error: "invalid short link",
			},
		)
		return
	}

	result, err := app.db.ExecContext(
		r.Context(),
		`
			DELETE FROM short_links
			WHERE code = $1
			AND user_id = $2
		`,
		code,
		userID,
	)

	if err != nil {
		log.Printf(
			"delete link %s for user %d: %v",
			code,
			userID,
			err,
		)

		writeJSON(
			w,
			http.StatusInternalServerError,
			errorResponse{
				Error: "could not delete link",
			},
		)
		return
	}

	rowsAffected, err := result.RowsAffected()

	if err != nil {
		log.Printf(
			"read delete result: %v",
			err,
		)

		writeJSON(
			w,
			http.StatusInternalServerError,
			errorResponse{
				Error: "could not delete link",
			},
		)
		return
	}

	if rowsAffected == 0 {
		writeJSON(
			w,
			http.StatusNotFound,
			errorResponse{
				Error: "link not found",
			},
		)
		return
	}

	writeJSON(
		w,
		http.StatusOK,
		map[string]any{
			"message": "link deleted",
		},
	)
}

func (app *application) redirect(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")

	if !validCode(code) {
		http.NotFound(w, r)
		return
	}

	var targetURL string

	// Update the click count and retrieve the URL in one database query.
	err := app.db.QueryRowContext(
		r.Context(),
		`
			UPDATE short_links
			SET clicks = clicks + 1
			WHERE code = $1
			RETURNING target_url
		`,
		code,
	).Scan(&targetURL)

	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}

	if err != nil {
		log.Printf("resolve short link: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// 302 allows to change the target later without browsers treating the redirect as permanently cached.
	http.Redirect(w, r, targetURL, http.StatusFound)
}

func normalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)

	if raw == "" {
		return "", errors.New("URL is required")
	}

	if len(raw) > 2048 {
		return "", errors.New("URL is too long")
	}

	// Improve usability: example.com becomes https://example.com.
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "", errors.New("URL is invalid")
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("only http and https URLs are allowed")
	}

	if parsed.Host == "" {
		return "", errors.New("URL must include a hostname")
	}

	return parsed.String(), nil
}

func generateCode(length int) (string, error) {
	result := make([]byte, length)
	alphabetLength := big.NewInt(int64(len(codeAlphabet)))

	for i := range result {
		position, err := rand.Int(rand.Reader, alphabetLength)
		if err != nil {
			return "", fmt.Errorf("generate random character: %w", err)
		}

		result[i] = codeAlphabet[position.Int64()]
	}

	return string(result), nil
}

func validCode(code string) bool {
	if len(code) != codeLength {
		return false
	}

	for _, character := range code {
		if !strings.ContainsRune(codeAlphabet, character) {
			return false
		}
	}

	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("encode JSON response: %v", err)
	}
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		next.ServeHTTP(w, r)

		log.Printf(
			"%s %s %s",
			r.Method,
			r.URL.Path,
			time.Since(start),
		)
	})
}

func envOrDefault(name, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}

	return value
}

func serverAddress() string {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	return "0.0.0.0:" + port
}

func (app *application) myLinksPage(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(
		w,
		r,
		"./webfiles/my-links.html",
	)
}
