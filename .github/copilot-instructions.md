# Synthezia AI Coding Instructions

## Copilot developer profile
- Senior Software Engineer
- Experienced in Go backend development with Gin framework
- Proficient in React and TypeScript for frontend development
- Familiar with Python integration and virtual environment management

## Project Overview
Synthezia is a self-hosted audio transcription service combining a **Go backend** (Gin framework) with a **React/TypeScript frontend** (Vite), packaged as a single binary. It orchestrates Python-based transcription (WhisperX, NVIDIA models) via embedded Python environments using `uv` package manager.

## Architecture

### Monolithic Binary with Embedded Frontend
- React frontend builds to `web/frontend/dist/`, then copied to `internal/web/dist/` for Go embedding
- Build sequence: `npm run build` → copy to `internal/web/` → `go build` (see `build.sh`)
- Frontend serves from embedded files (checked into git at `internal/web/dist/`)

### Backend Structure
```
cmd/server/main.go              # Entry point, initializes services
internal/
  api/                          # Gin HTTP handlers & routing
    router.go                   # Route definitions, middleware stack
    handlers.go                 # Core API handlers (3k+ lines)
    *_handlers.go              # Feature-specific handlers
  transcription/               # Plugin-based transcription system
    interfaces/                # Core adapter interfaces
    registry/                  # Auto-discovery & model selection
    adapters/                  # WhisperX, Parakeet, Canary, PyAnnote, Sortformer
    unified_service.go         # Main orchestrator
    queue_integration.go       # Legacy queue compatibility
  queue/                       # Job queue with auto-scaling workers
  auth/                        # JWT + API key authentication
  database/                    # GORM + SQLite with WAL mode
  llm/                         # Ollama/OpenAI abstraction layer
  config/                      # ENV-based config with .env fallback
  models/                      # GORM models (transcription.go, auth.go, etc.)
pkg/
  logger/                      # Structured logging wrapper (slog)
  middleware/                  # Auth, compression, CORS
```

### Transcription System (Plugin Architecture)
**Critical**: New extensible system (see `internal/transcription/README.md`) allows hot-plugging models via auto-registration:
1. Models implement `TranscriptionAdapter` or `DiarizationAdapter` from `interfaces/`
2. `init()` function registers with `registry/` on import
3. `UnifiedJobProcessor` orchestrates via `unified_service.go`
4. Python environments auto-bootstrap on first use (uv + virtual envs)

**Example**: Adding a model requires only creating `adapters/mymodel_adapter.go` with `init()` registration—no changes to core services.

## Developer Workflows

### Local Development
```bash
# Frontend (React + Vite)
cd web/frontend
npm install
npm run dev          # Dev server on :5173 with hot reload

# Backend
go run cmd/server/main.go  # Starts on :8080, serves embedded frontend

# Full build
./build.sh           # Builds frontend → Go binary with embedded assets
./synthezia          # Run the binary
```

### Testing
```bash
./run_tests.sh      # Orchestrates all backend tests (security, auth, database, etc.)
go test ./tests/security_test.go    # Individual test suite
cd web/frontend && npm run lint      # Frontend linting
```

### Docker Builds
- `Dockerfile`: Multi-stage (Node → Go → Python runtime with uv + ffmpeg)
- `docker-compose.yml`: Simple deployment
- `docker-compose.cuda.yml`: GPU-accelerated variant
- Entrypoint script handles PUID/PGID for volume permissions

### API Documentation
- Swagger annotations in handlers (e.g., `@Summary`, `@Param`)
- Generate with: `swag init -g cmd/server/main.go`
- Served at `/swagger/index.html`

## Critical Patterns

### Authentication Flow
- **JWT**: User sessions, required for account/API key management (see `middleware.JWTOnlyMiddleware`)
- **API Keys**: Programmatic access, allowed for most endpoints (see `middleware.AuthMiddleware`)
- Dual middleware: `JWTOnlyMiddleware` for user-specific routes, `AuthMiddleware` for API + JWT

### Database Conventions
- SQLite with WAL mode + aggressive pragmas (`database.go` lines 27-38)
- GORM models in `internal/models/` with embedded structs (e.g., `WhisperXParams` embedded in `TranscriptionJob`)
- Migration handled by `AutoMigrate()` in `database.Initialize()`

### Queue System
- `TaskQueue` in `internal/queue/` manages concurrent transcription jobs
- Auto-scaling workers (min/max configurable)
- `JobProcessor` interface allows swapping implementations (legacy vs unified)
- Tracks running OS processes for cancellation (`RunningJob` struct with `*exec.Cmd`)

### Python Environment Management
- `uv` (Astral's package manager) replaces pip for speed
- Environments stored at `data/whisperx-env/` (or `WHISPERX_ENV`)
- `findUVPath()` in `config/config.go` auto-detects uv on macOS/Linux
- Models bootstrap their own dependencies via `InitEmbeddedPythonEnv()`

### Logging
- Structured logging via `pkg/logger/` (wraps slog)
- Startup logs use `logger.Startup("component", "message")` for clarity
- Set `LOG_LEVEL=debug` for verbose output

### Frontend Integration
- API calls to `/api/v1/*` (no proxy needed—single binary serves all)
- TailwindCSS v4 + Radix UI components + shadcn/ui patterns
- WaveSurfer.js for audio playback, React Markdown for transcript rendering
- Build output manually chunked for vendor caching (see `vite.config.ts`)

## Common Tasks

### Adding a New API Endpoint
1. Add handler in `internal/api/*_handlers.go`
2. Register route in `router.go` (mind middleware: auth vs no-auth, compression)
3. Add Swagger annotations: `@Summary`, `@Tags`, `@Param`, `@Success`, `@Failure`
4. Run `swag init` to regenerate docs

### Adding a Transcription Model
1. Create `internal/transcription/adapters/mymodel_adapter.go`
2. Implement `TranscriptionAdapter` or `DiarizationAdapter`
3. Define `ModelCapabilities` and `ParameterSchema` in adapter constructor
4. Add `init()` function calling `registry.RegisterTranscriptionAdapter()`
5. Import in `cmd/server/main.go` (or rely on `_ "synthezia/internal/transcription/adapters"`)
6. Model auto-appears in UI and API

### Database Schema Changes
1. Modify structs in `internal/models/`
2. `go run cmd/server/main.go` triggers AutoMigrate on startup
3. For complex migrations, add SQL in `database.Initialize()` after AutoMigrate

## Configuration
Environment variables (`.env` or system):
```bash
PORT=8080
HOST=localhost
DATABASE_PATH=./data/synthezia.db
UPLOAD_DIR=./data/uploads
WHISPERX_ENV=./data/whisperx-env
JWT_SECRET=<auto-generated-if-missing>
LOG_LEVEL=info
UV_PATH=/usr/local/bin/uv  # Optional: override uv location
```

## Gotchas
- **Frontend embedding**: Must run `build.sh`, not just `go build` (assets not embedded otherwise)
- **Middleware order**: Compression before auth, `NoCompressionMiddleware` for uploads (see `router.go` lines 29-92)
- **Job cancellation**: Must handle both context cancellation AND OS process termination (see `queue.TaskQueue.CancelJob`)
- **SQLite concurrency**: WAL mode required for multi-writer support
- **Python model downloads**: First transcription triggers large model downloads (e.g., WhisperX models ~500MB-3GB)
