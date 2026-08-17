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
		log.Println("No .env file found; using system environment variables")
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

	app := &application{
		db:                db,
		redisClient:       redisClient,
		baseURL:           baseURL,
		dummyPasswordHash: dummyPasswordHash,
	}

	// router
	mux := http.NewServeMux()

	// API
	mux.HandleFunc("POST /api/links", app.createLink)
	mux.HandleFunc("POST /api/auth/register", app.register)
	mux.HandleFunc("POST /api/auth/login", app.login)

	// frontend
	mux.HandleFunc("GET /{$}", app.home)
	mux.Handle(
		"GET /static/",
		http.StripPrefix(
			"/static/",
			http.FileServer(http.Dir("./web/static")),
		),
	)

	mux.HandleFunc("GET /{code}", app.redirect)
	mux.HandleFunc("GET /register", app.registerPage)
	mux.HandleFunc("GET /login", app.loginPage)

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
	http.ServeFile(w, r, "./web/register.html")
}

func (app *application) loginPage(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "./web/login.html")
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
			writeJSON(w, http.StatusAccepted, map[string]any{
				"message": "if this email can be registered, check your inbox for the next step",
			})
			return
		}
		log.Printf("insert user: %v", err)

		writeJSON(w, http.StatusInternalServerError, errorResponse{
			Error: "could not create account",
		})
		return
	}
}

func (app *application) home(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "./web/index.html")
}

func (app *application) createLink(w http.ResponseWriter, r *http.Request) {
	// Prevent clients from sending an unexpectedly large request body.
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB

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

	// Retry in the unlikely event that a generated code already exists.
	for attempt := 0; attempt < 5; attempt++ {
		code, err := generateCode(codeLength)
		if err != nil {
			log.Printf("generate code: %v", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{
				Error: "could not create short link",
			})
			return
		}

		var insertedCode string

		err = app.db.QueryRowContext(
			r.Context(),
			`
				INSERT INTO short_links (code, target_url)
				VALUES ($1, $2)
				ON CONFLICT (code) DO NOTHING
				RETURNING code
			`,
			code,
			targetURL,
		).Scan(&insertedCode)

		switch {
		case err == nil:
			writeJSON(w, http.StatusCreated, createLinkResponse{
				Code:     insertedCode,
				ShortURL: app.baseURL + "/" + insertedCode,
				LongURL:  targetURL,
			})
			return

		case errors.Is(err, sql.ErrNoRows):
			// The generated code collided with an existing code.
			continue

		default:
			log.Printf("insert short link: %v", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{
				Error: "could not save short link",
			})
			return
		}
	}

	writeJSON(w, http.StatusInternalServerError, errorResponse{
		Error: "could not generate a unique short link",
	})
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
