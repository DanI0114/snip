# Snip
Snip is a full-stack URL shortener built with Go, PostgreSQL, Redis/Valkey, and vanilla JavaScript.
It allows users to create an account, verify their email address, shorten URLs, track click counts, manage their links, and delete links they no longer need.
The project was built to explore backend architecture, authentication, session management, database design, and production deployment without relying on a frontend framework.

## Live Demo
[View Snip](https://snip-mm8n.onrender.com)

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
- PostgreSQL
- Redis/Valkey

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


### Infrastructure
- Docker
- Docker Compose
- Render Web Service
- Render PostgreSQL
- Render Key Value / Valkey
- Brevo transactional email

---

## QA Automation

Snip includes an automated QA suite built with **Python, Pytest, and Playwright**, covering both the REST API and browser UI.

The current suite contains **34 automated tests** focused on functional behavior, validation, authentication, authorization, session management, and end-to-end application behavior.

### What the 34 Automated Tests Cover

#### Registration API
- Minimum and maximum password-length boundaries
- Password confirmation mismatch
- Invalid email formats
- Empty and whitespace-only names
- Maximum name length
- Unknown JSON fields
- Malformed JSON requests
- Missing required fields
- Expected HTTP status codes and API error responses

#### Authentication & Session API
- Successful login with valid credentials
- Rejection of incorrect passwords
- Rejection of nonexistent users
- Authenticated access to `/api/auth/me`
- Rejection of unauthenticated access to protected endpoints
- Logout behavior
- Session invalidation after logout
- Generic authentication errors to reduce account-enumeration risk
- Malformed login requests
- Missing credential handling

#### Short-Link API
- Authenticated short-link creation
- Rejection of unauthenticated link creation
- Empty URL validation
- Unsupported URL schemes
- Missing hostnames
- Maximum URL length
- Automatic HTTPS normalization
- HTTP `302` redirect behavior
- Deletion of owned links
- Invalid short-link codes
- Verification that deleted links are no longer accessible

#### Authorization
Object-level authorization is tested using two separate authenticated users:

1. **User A** creates a short link.
2. **User B** attempts to delete User A's link.
3. The deletion request is rejected.
4. User A's original link remains accessible.

This verifies that knowing another resource's identifier is not enough to modify or delete it.

#### UI Automation
- Registration page availability
- Important registration form elements
- Password-confirmation validation through the browser

### Test Design Techniques Used
- Positive testing
- Negative testing
- Boundary-value analysis
- Input-validation testing
- API contract testing
- Authentication testing
- Authorization testing
- Session-state testing
- Security-oriented testing
- Test isolation
- Pytest parameterization
- UI smoke testing
- End-to-end behavior verification

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
```
---
## What I Learned
I created Snip to learn about backend tech and understand how a web application works in general 

Some of the main concepts i learned in this project were:
- Designing HTTP APIs in Go
- Working with PostgreSQL from Go
- Modeling relationships between users and application data
- Implementing authentication without JWTs
- Building server-side session management with Redis
- Secure password storage with Argon2id
- Email verification flows
- Protecting user-owned resources
- Handling frontend authentication state
- Dockerizing a Go application
- Managing production environment variables
- Migrating a production database
- Deploying PostgreSQL, Valkey, and a Go web service to Render
