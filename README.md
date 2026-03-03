# Code Nest

A cloud-based IDE platform for creating and managing containerized development workspaces. Users authenticate via email/password or OAuth (Google/GitHub), create projects linked to GitHub repositories, and get access to live workspaces with a remote terminal and file editor

It is a cloud development platform similar to github codespaces.

---

## Architecture

Code Nest is a Go microservices monorepo with gRPC for internal communication and a REST gateway for external clients.

```
Client
  │
  ▼
Gateway (HTTP :3000)          ← Gin REST API, auth middleware
  ├── Auth Service (gRPC :50051)   ← JWT, OAuth2 (Google/GitHub), JWKS
  └── Project Service (gRPC :50052) ← Project/workspace lifecycle

Agent (:9000)                 ← Runs inside each workspace container
  ├── WebSocket /terminal     ← Interactive shell
  └── HTTP /files             ← File browser, read, save
```

Shared infrastructure: **PostgreSQL 16** (two databases) · **Redis 7** (session caching, ownership TTL)

---

## Tech Stack

| Layer | Technology |
|---|---|
| Language | Go 1.24 |
| HTTP / Routing | Gin |
| Service RPC | gRPC + Protocol Buffers |
| Auth | JWT (RSA-256), OAuth2 (Google, GitHub App) |
| Database | PostgreSQL 16 via GORM + gormigrate |
| Cache | Redis 7 |
| Real-time | Gorilla WebSocket |
| Containers | Docker + Docker Compose |

---

## Project Structure

```
code_nest/
├── agent/                      # Workspace runtime agent (terminal, files, git)
├── services/
│   ├── auth-service/           # gRPC auth: signup, login, OAuth, JWT, JWKS
│   ├── gateway/                # HTTP API gateway (Gin), routes to gRPC services
│   └── project-service/        # gRPC project/workspace lifecycle management
├── proto/                      # Shared protobuf definitions
├── tests/                      # Integration & E2E test suites
├── scripts/
│   └── init-databases.sh       # PostgreSQL DB initialization
├── docker-compose.yml
└── .env.example
```

---

## Getting Started

### Prerequisites
- Docker & Docker Compose
- A Google OAuth app (Client ID + Secret)
- A GitHub OAuth / GitHub App (App ID, private key, slug)

### Run with Docker Compose

```bash
cp .env.example .env
# Fill in secrets in .env (see Configuration below)
docker-compose up -d
```

Gateway is available at `http://localhost:3000`.

### Local Development (per service)

```bash
# Auth service
cd services/auth-service && go run cmd/auth-service/main.go

# Project service
cd services/project-service && go run cmd/project-service/main.go

# Gateway
cd services/gateway && go run cmd/gateway/main.go

# Agent
cd agent && go run cmd/agent/main.go
```

Database migrations run automatically on startup via gormigrate.

---

## Configuration

Copy `.env.example` to `.env` and set the following:

| Variable | Description |
|---|---|
| `JWT_SECRET` | 32-char random string for JWT signing |
| `GOOGLE_CLIENT_ID / SECRET / REDIRECT_URL` | Google OAuth2 app credentials |
| `GITHUB_APP_ID / SLUG / PRIVATE_KEY_PATH` | GitHub App credentials |
| `AUTH_POSTGRES_USER/PASSWORD/DB` | Auth service DB credentials |
| `PROJECT_POSTGRES_USER/PASSWORD/DB` | Project service DB credentials |
| `REDIS_ADDR` | Redis address (default `redis:6379`) |
| `ATLAS_BASE_URL` | URL to the workspace provisioner |
| `INTERNAL_WEBHOOK_SECRET` | Secret for agent→gateway webhook calls |

---

## API Reference

### Gateway (`:3000`)

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| GET | `/api/health` | — | Health check |
| POST | `/api/auth/signup` | — | Register (email + password) |
| POST | `/api/auth/login` | — | Login, returns JWT |
| GET | `/api/auth/google/url` | — | Google OAuth redirect URL |
| GET | `/api/auth/google/callback` | — | Google OAuth callback |
| GET | `/api/auth/github/url` | — | GitHub OAuth redirect URL |
| GET | `/api/auth/github/callback` | — | GitHub OAuth callback |
| POST | `/api/projects` | Bearer | Create project |
| POST | `/api/projects/:id/start` | Bearer | Start workspace |
| GET | `/auth/verify` | Bearer | Token verification (reverse proxy) |
| POST | `/api/internal/webhook` | Token | Agent status callback |

### Agent (`:9000`)

| Method | Endpoint | Description |
|---|---|---|
| GET | `/health` | Agent health |
| WS | `/terminal` | Interactive shell (WebSocket) |
| GET | `/files` | List directory |
| GET | `/files/content` | Read file |
| POST | `/files/save` | Write file |

### Auth Service gRPC (`:50051`)

`Signup` · `Login` · `GetGoogleAuthURL` · `HandleGoogleCallback` · `GetGitHubAuthURL` · `HandleGitHubCallback` · `ValidateToken` · `GetGitHubAccessToken`

JWKS endpoint: `GET http://auth-service:8081/.well-known/jwks.json`

### Project Service gRPC (`:50052`)

`CreateProject` · `StartWorkspace` · `StopWorkspace` · `VerifyAndComplete` · `IsOwner`

---

## Testing

```bash
cd tests/
go run basic_integration_test.go    # Auth & project flows
go run e2e_test.go                   # End-to-end scenarios
go run agent_integration_test.go     # Agent workspace tests
```
