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
	"net/url"
	"os"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
)

const (
	defaultAddress = ":8080"
	codeLength     = 7
	codeAlphabet   = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
)

type application struct {
	db      *sql.DB
	baseURL string
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

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	// sql.Open does not necessarily establish a connection immediately,
	// so explicitly verify the database at startup.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("connect to database: %v", err)
	}

	// Keep the pool small for the MVP.
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxIdleTime(5 * time.Minute)

	app := &application{
		db:      db,
		baseURL: baseURL,
	}

	// Creating a router
	mux := http.NewServeMux()

	// API.
	mux.HandleFunc("POST /api/links", app.createLink)

	// Frontend.
	mux.HandleFunc("GET /{$}", app.home)
	mux.Handle(
		"GET /static/",
		http.StripPrefix(
			"/static/",
			http.FileServer(http.Dir("./web/static")),
		),
	)

	// Short links, for example: https://example.com/aB92xQz
	mux.HandleFunc("GET /{code}", app.redirect)

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

	// 302 allows you to change the target later without browsers treating the
	// redirect as permanently cached.
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
