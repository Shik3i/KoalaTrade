# Contributing to KoalaTrade

Thanks for your interest in contributing! This guide covers how to set up the
project, the conventions we follow, and how to get a change merged.

By participating you agree to abide by our [Code of Conduct](CODE_OF_CONDUCT.md).

## Prerequisites

- **Go** 1.26+
- **Node.js** 22+ and npm
- **Docker** (optional, for full-stack runs)

## Setup

```bash
git clone https://github.com/Shik3i/KoalaTrade.git
cd KoalaTrade
cp .env.example .env

# Backend
cd backend && go run ./cmd/server      # http://127.0.0.1:8080

# Frontend (separate terminal)
cd frontend && npm install && npm run dev   # http://127.0.0.1:5173
```

The frontend dev server proxies `/api` to the backend. Override the target with
`KOALA_API_TARGET` if your backend runs on another port.

## Before you open a PR

Run the same checks CI runs:

```bash
# Backend
cd backend && go build ./... && go test ./... && gofmt -l .

# Frontend
cd frontend && npm run check && npm run build
```

- `gofmt -l .` should print nothing (run `gofmt -w .` to fix).
- `npm run check` (svelte-check) must report **0 errors, 0 warnings**.

## Conventions

- **Commits** follow [Conventional Commits](https://www.conventionalcommits.org/):
  `feat:`, `fix:`, `chore:`, `docs:`, `refactor:`, `test:` — with an optional scope, e.g. `feat(backend): ...`.
- **Branches**: never commit directly to `main`. Use `feat/...`, `fix/...`, `docs/...` and open a PR.
- **Go**: standard library first; keep packages under `backend/internal/`. Handlers stay thin, logic in packages.
- **Frontend**: Svelte 5, TypeScript, local CSS variables. No new runtime dependencies without discussion, and **no CDN / remote fonts / analytics** (privacy-first).
- **Secrets**: never commit `.env` or real API keys. Server owns all third-party API traffic.

## Architecture & docs

Skim [docs/architecture.md](docs/architecture.md) before larger changes. Update
the relevant docs and [CHANGELOG.md](CHANGELOG.md) (`Unreleased` section) with
user-facing changes.

## Pull requests

1. Keep PRs focused and describe the change + how you verified it.
2. Ensure CI is green (tests, checks, and Docker image builds run automatically).
3. A maintainer reviews and merges. PRs are typically squash-merged into `main`.

## Releases

Releases are cut by tagging `main`:

```bash
git tag v0.1.0 && git push origin v0.1.0
```

This triggers the Docker Release workflow, which builds and publishes the backend
and frontend images to GHCR. Update `CHANGELOG.md` before tagging.
