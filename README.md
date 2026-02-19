# Go File Explorer

REST API backend in Go for browsing, uploading, downloading, managing, and searching files in a sandboxed storage root.

## Features

- Directory listing and creation
- File upload, download, preview, metadata info, and directory ZIP download
- Rename, move, copy, soft-delete, and restore operations
- Recursive search with filters and pagination
- JWT authentication with role-based authorization
- Security hardening: recovery, logging, CORS, rate limiting, security headers, request timeout
- Structured audit logging for write operations (who, what, when, IP, before/after)

## Requirements

- Go 1.24+
- Optional: Docker + Docker Compose

## Quick Start (Local)

1. Copy environment file:

   - Windows PowerShell:
     ```powershell
     Copy-Item .env.example .env
     ```
   - Bash:
     ```bash
     cp .env.example .env
     ```

2. Set `JWT_SECRET` in `.env`.

3. Run:

   ```bash
   go mod tidy
   go run ./cmd/server
   ```

Server starts on `http://localhost:8080`.

Health endpoint:

```http
GET /health
```

## Default User

On first start, the service seeds a default admin user in `USERS_FILE`.

- Username: `admin`
- Password: `admin123`

Change this immediately in non-test environments.

## Authentication

Login and use the returned access token in `Authorization: Bearer <token>`.

```http
POST /api/v1/auth/login
```

Protected route example:

```http
GET /api/v1/files?path=/
Authorization: Bearer <access_token>
```

### curl Examples

Login and capture tokens:

```bash
curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}'
```

List root directory (replace `ACCESS_TOKEN`):

```bash
curl -s "http://localhost:8080/api/v1/files?path=/&page=1&limit=50" \
  -H "Authorization: Bearer ACCESS_TOKEN"
```

Upload a file:

```bash
curl -s -X POST http://localhost:8080/api/v1/files/upload \
  -H "Authorization: Bearer ACCESS_TOKEN" \
  -F "path=/uploads" \
  -F "files=@./example.txt"
```

Search PDF files:

```bash
curl -s "http://localhost:8080/api/v1/search?q=report&path=/documents&type=file&ext=.pdf&page=1&limit=20" \
  -H "Authorization: Bearer ACCESS_TOKEN"
```

## Endpoint Summary

- Auth
  - `POST /api/v1/auth/login`
  - `POST /api/v1/auth/register` (admin)
  - `POST /api/v1/auth/refresh`
  - `POST /api/v1/auth/logout`
  - `GET /api/v1/auth/me`

- Directory + Files
  - `GET /api/v1/files`
  - `POST /api/v1/directories`
  - `POST /api/v1/files/upload`
  - `GET /api/v1/files/download`
  - `GET /api/v1/files/preview`
  - `GET /api/v1/files/info`

- Management
  - `PUT /api/v1/files/rename`
  - `PUT /api/v1/files/move`
  - `POST /api/v1/files/copy`
  - `DELETE /api/v1/files` (soft delete to trash)
  - `POST /api/v1/files/restore`

- Search
  - `GET /api/v1/search?q=...&path=...&type=file|dir&ext=.pdf&page=1&limit=20`

## Tests

```bash
go test ./internal/... -v
go test ./test/integration/... -v -tags=integration
```

Or with make:

```bash
make test-all
```

Full endpoint E2E script (requires running server):

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\test-all-endpoints.ps1
```

Or via make:

```bash
make test-endpoints
```

## Docker

1. Create `.env` from `.env.example`.
2. Run:

```bash
docker compose up --build -d
```

Data persists in:

- `./data` (storage)
- `./state` (users DB)

Stop:

```bash
docker compose down
```