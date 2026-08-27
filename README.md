# Snip
Snip is a full-stack URL shortener built with Go, PostgreSQL, Redis/Valkey, and vanilla JavaScript.

It allows users to create an account, verify their email address, shorten URLs, track click counts, manage their links, and delete links they no longer need.

The project was built to explore backend architecture, authentication, session management, database design, and production deployment without relying on a frontend framework.

## Live Demo
[View Snip](https://snip-mm8n.onrender.com)

> The application is hosted on Render, so the free instance may take a few seconds to wake up after a period of inactivity.

---

## Features

### URL Shortening

- Generate short, unique URLs
- Redirect short URLs to their original destination
- Track the number of clicks for each link
- Copy shortened URLs directly from the interface

### Authentication

- User registration
- Argon2id password hashing
- Email verification
- Secure login and logout
- Server-side sessions stored in Redis/Valkey
- HttpOnly session cookies
- Multiple-session support

### User Dashboard

Authenticated users can:

- View all of their shortened links
- See click counts
- See when each link was created
- Copy shortened URLs
- Delete their own links

Link ownership is enforced on the server using the authenticated user's ID.

---

## Tech Stack

### Backend

- Go
- `net/http`
- PostgreSQL
- `database/sql`
- pgx
- Redis / Valkey
- go-redis

### Authentication & Security

- Argon2id password hashing
- Cryptographically secure session tokens
- SHA-256 session token fingerprints
- Server-side session storage
- HttpOnly cookies
- SameSite cookies
- Email verification tokens with expiration

### Frontend

- HTML
- CSS
- Vanilla JavaScript
- Fetch API

### Infrastructure

- Docker
- Docker Compose
- Render Web Service
- Render PostgreSQL
- Render Key Value / Valkey
- Brevo transactional email

---

## Architecture

Snip uses PostgreSQL for persistent application data and Redis/Valkey for temporary authentication state.

```text
Browser
   |
   v
Go HTTP Server
   |
   +--------------------+
   |                    |
   v                    v
PostgreSQL          Redis / Valkey
   |                    |
   |                    |
Users                Sessions
Short links           Verification tokens
Click counts