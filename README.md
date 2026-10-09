# MMSU Tennis Club

A starter monorepo for the MMSU Tennis Club web application. The first version is intentionally small: a Go API health endpoint and a Next.js landing page. Training and booking features will be added in later tasks.

## Repository structure

```text
apps/
  api/                 Go HTTP API
  web/                 Next.js website
supabase/
  migrations/          Future version-controlled SQL migrations
docs/                  Project notes
```

## What the starter apps do

- **Go API (`apps/api`)** runs an HTTP server with `GET /health` and `GET /api/training-sessions`. The schedule endpoint reads upcoming public sessions directly from Supabase PostgreSQL using `pgx` and `DATABASE_URL`.
- **Next.js app (`apps/web`)** serves a simple MMSU Tennis Club landing page. It does not call the API yet.
- **Supabase** provides managed PostgreSQL as the application's source of truth. Schema changes are versioned as SQL migration files under `supabase/migrations/` and applied in order.
- **Cal.com** may later provide the scheduling/booking interface. The application will retain its own important booking records in PostgreSQL; Cal.com is not configured in this starter.

## Requirements

- Go 1.26 or compatible recent Go version
- Node.js 20.9 or newer and npm

## Run the Go API

In PowerShell, from the repository root:

```powershell
cd apps/api
go run ./cmd/api
```

The API listens on `http://localhost:8080` by default. Check it in a browser or another terminal:

```powershell
Invoke-RestMethod http://localhost:8080/health
```

To choose a different port, set `PORT` before starting the API, for example `$env:PORT = "9000"`.

## Run the Next.js app

In another PowerShell terminal, from the repository root:

```powershell
cd apps/web
npm install
npm run dev
```

Open `http://localhost:3000` in your browser. `npm install` downloads the dependencies listed in `apps/web/package.json` and creates a local lockfile; it does not change application source code.

## Environment variables and secrets

Copy `apps/api/.env.example` to `apps/api/.env` and `apps/web/.env.example` to `apps/web/.env.local` when you need local settings. The API reads `PORT` and `DATABASE_URL` directly from the process environment; the example files are not automatically loaded. The web example documents the future API URL; the landing page does not use it yet.

Never put real passwords, API keys, or database credentials in source code or commit them to Git. Local environment files are ignored by `.gitignore`. Only values deliberately prefixed with `NEXT_PUBLIC_` are suitable for browser-visible configuration; never put secrets in them.

### Training schedule database access

The public schedule API uses the dedicated `mmsu_schedule_reader` group role so database access is limited to the columns and rows required by the schedule query. The follow-up migration creates this role as `NOLOGIN`; it does not create a user or password.

Do not use the Supabase `postgres` owner credentials as the permanent application credential. The owner can bypass RLS and has broad schema privileges. After the role migration has been reviewed and applied, create a separate restricted login role and grant it membership only in `mmsu_schedule_reader` (with membership inheritance enabled) so it inherits the reader permissions. Do not grant it membership in `postgres`, `service_role`, or another privileged role. The separate login also needs database `CONNECT`; this migration intentionally grants no database-level privileges. Configure that login's password securely outside Git and set `DATABASE_URL` to the direct PostgreSQL connection details from the Supabase Dashboard. Do not put a real connection string or password in source control or frontend configuration.

The schedule role has no booking write privileges. Any future booking-write path requires a separate security review and permission design.

## Future database changes

When database work begins, add each schema change as a new, ordered SQL migration file under `supabase/migrations/`. Do not edit an already-applied migration to change a deployed schema; create a later migration instead. This makes the database history reviewable alongside the application code.
