# AGENTS.md

## Repository Scope

These instructions apply to the entire `mmonitor` repository.

## Project Overview

- `mmonitor` is a single-binary pure-Go (`CGO_ENABLED=0`) multi-backend Mihomo traffic monitoring service with embedded SQLite (`modernc.org/sqlite`) and an embedded Shadcn UI (`base-nova`) React frontend.
- Backend packages live under `internal/` (`config`, `icon`, `mihomo`, `storage`, `monitor`, `server`) with platform entrypoints in `main.go`, `platform_windows.go`, and `platform_headless.go`.
- Canonical brand logo lives at `assets/logo.svg` (used for both browser `/favicon.svg` and runtime-generated Windows tray ICO via `internal/icon`).
- Web frontend source lives in `web/` (`pnpm` + Vite + React + TypeScript + Tailwind CSS v4 + Shadcn UI `base-nova` + TanStack Query + Recharts + i18next) and builds into `web/dist`, which Go embeds via `//go:embed LICENSE assets/logo.svg all:web/dist`.
- Documentation screenshots and assets live in `docs/`.

## Working Agreements

- Keep changes focused and preserve the single-binary pure-Go architecture (`CGO_ENABLED=0`).
- Use Mihomo terminology consistently (`mihomo`, `mmonitor`); do not reintroduce `clash` or `traffic-monitor` naming.
- Keep all text files on LF line endings (`.gitattributes`).
- Frontend base controls must come from Shadcn UI `base-nova` (`web/src/components/ui/*`), use semantic CSS tokens (`success`, `info`, `warning`, `destructive`), fetch server state via TanStack Query, and keep all UI copy in i18n locales (`zh-CN.json` / `en-US.json`).

## Validation

```bash
# Web frontend format check, typecheck & build (outputs to web/dist)
cd web && pnpm format:check && pnpm typecheck && pnpm build

# Backend tests & static checks (zero CGO)
CGO_ENABLED=0 go vet ./...
CGO_ENABLED=0 go test ./...
```

## Release Versioning

- Push `vX.Y.Z` tags to trigger `.github/workflows/release.yml`, which injects `mmonitor/internal/config.AppVersion` via `-ldflags`.
