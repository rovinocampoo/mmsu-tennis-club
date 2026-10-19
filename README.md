# MMSU Tennis Club

A monorepo for the MMSU Tennis Club web application, with a Go API, public training schedules and manual booking approval, plus a coach directory, a read-only coach portal, and admin schedule management.

## Repository structure

```text
apps/
  api/                 Go HTTP API
  web/                 Next.js website
supabase/
  migrations/          Future version-controlled SQL migrations
docs/                  Project notes
```

## What the apps do

- **Go API (`apps/api`)** serves health, public training schedules, booking requests, suggestions/feedback submissions, the read-only coach schedule and directory, and protected admin scheduling/review/feedback/coach-directory endpoints.
- **Next.js app (`apps/web`)** serves the customer training schedule in calendar and chronological list views, a club FAQ at `/faq`, a suggestions and feedback form at `/suggestions`, the read-only coach portal at `/coach`, and admin booking/schedule management, a coach directory at `/admin/coaches`, and a feedback inbox at `/admin`.
- **Supabase** provides managed PostgreSQL as the application's source of truth. Schema changes are versioned as SQL migration files under `supabase/migrations/` and applied in order.
- **Cal.com** may later provide the scheduling/booking interface. The application will retain its own important booking records in PostgreSQL; Cal.com is not configured in this starter.

## Requirements

- Go 1.26 or compatible recent Go version
- Node.js 20.9 or newer and npm

## Run the Go API

In PowerShell, from the repository root:

```powershell
cd apps/api
$env:API_HOST = "127.0.0.1"
$env:PORT = "8081"
go run ./cmd/api
```

The API listens on `http://localhost:8080` by default. `API_HOST` is optional; when unset, the server keeps its existing all-interface bind behavior. For local testing, set it to `127.0.0.1` as shown above. Check the local API in a browser or another terminal:

```powershell
Invoke-RestMethod http://127.0.0.1:8081/health
```

To choose a different port, set `PORT` before starting the API, for example `$env:PORT = "9000"`. `API_HOST` accepts a host name or IP address; the server combines it with `PORT` using Go's address formatting, including IPv6 hosts.

## Run the Next.js app

In another PowerShell terminal, from the repository root:

```powershell
cd apps/web
npm install
npm run dev
```

Open `http://localhost:3000` in your browser. `npm install` downloads the dependencies listed in `apps/web/package.json` and creates a local lockfile; it does not change application source code.

## Environment variables and secrets

Copy `apps/api/.env.example` to `apps/api/.env` and `apps/web/.env.example` to `apps/web/.env.local` when you need local settings. The API reads its configuration directly from the process environment; the example files are not automatically loaded. The web server uses `API_BASE_URL` for its Go API proxy, and sign-in pages use the public Supabase URL and publishable key.

Never put real passwords, API keys, or database credentials in source code or commit them to Git. Local environment files are ignored by `.gitignore`. Only values deliberately prefixed with `NEXT_PUBLIC_` are suitable for browser-visible configuration; never put secrets in them.

### Training schedule database access

The public schedule API uses the dedicated `mmsu_schedule_reader` group role so database access is limited to the columns and rows required by the schedule query. The follow-up migration creates this role as `NOLOGIN`; it does not create a user or password.

Do not use the Supabase `postgres` owner credentials as an application credential. The owner can bypass RLS and has broad schema privileges. After the role migrations have been reviewed and applied, provision one restricted login for `DATABASE_URL` with membership only in `mmsu_schedule_reader` and `mmsu_booking_api`, and a separate restricted login for `ADMIN_DATABASE_URL` with membership only in `mmsu_booking_admin_api`. Enable membership inheritance and grant each login database `CONNECT`; the migrations intentionally grant no database-level privileges. Never grant an application login membership in either function-owner role, `postgres`, `service_role`, or another privileged role. Configure passwords outside Git and keep both connection strings server-side. Do not put a real connection string in source control or frontend configuration.

The schedule role has no booking write privileges. The public booking API role can submit requests and read the shared coach schedule/directory through the protected API. The separate admin API role can call review, actual-session scheduling, coach-assignment, and coach-directory functions. Public booking functions execute as the dedicated non-login `mmsu_booking_function_owner`; admin session-management and coach/schedule-read functions execute as `mmsu_schedule_admin_function_owner`. Neither function-owner role may be granted to either application login.

### Coach directory, coach portal, and published training sessions

Coaches sign in at `/coach` using the provisioned Supabase Auth account(s) configured in `COACH_USER_IDS`; an authenticated account that is not on the coach allowlist cannot open the portal. The portal is read-only: it shows the whole-club schedule (the single assigned coach and each session's confirmed participant roster) and the active coach directory. Coaches no longer submit availability. Admins use the separate `ADMIN_USER_IDS` allowlist and `ADMIN_DATABASE_URL`.

Coach records live in the admin-managed `coaches` directory and are independent of any login. Admins add, rename, deactivate, or reactivate coaches at `/admin/coaches`; existing coach-availability rows are preserved as historical records but are no longer required, and the admin schedule loads them lazily only when the reference panel is opened. Admins then create actual sessions with a program, date/time, venue, capacity, and publication setting in the Schedule management view. A session may be assigned a single active coach from the directory (the assignment stores the coach name as a denormalized snapshot); the assignment runs under the club-wide schedule lock and refuses a coach who would overlap another active session. Admins can close/reopen registration, cancel sessions, change publication, and maintain notes. From the calendar view they can also publish every hidden, still-open session for the selected Manila day or month in one confirmed step through `POST /api/admin/training-sessions/publish`; eligibility is re-checked in the database, already-published sessions are left alone, and closed or cancelled sessions are never reopened or reactivated. Customers only see public sessions; the training page lists active `training_programs` alongside published sessions and does not let customers choose a coach or venue. The current program model has no package tiers or prices.

### Manual booking approval

Public submissions to `POST /api/booking-requests` create a `pending` request and return a public reference plus a one-time private status token. The browser keeps the token in the `/booking-status` URL fragment and submits it in the body of `POST /api/booking-status`; fragments are not sent as normal URL paths or query parameters. The database stores only its SHA-256 hash, and the token expires after 30 days. Status responses contain only the request reference, decision state, training program, session time, location, and next steps. A pending request is not a reservation and does not use session capacity. The form collects one name per participant (1–20 people): each name is stored as its own normalized `booking_request_participants` row while the comma-joined names remain in `booking_requests.full_name` so existing records and reports keep working. Email and phone are independently optional; a name-only request is accepted, and duplicate pending submissions are rejected per session by matching contact details and participant names. Admins sign in at `/admin` with an owner-provisioned Supabase Auth email/password account. There is no public admin registration. Every admin API call is independently verified by the Go API through the Supabase Auth `/auth/v1/user` endpoint, then checked against the server-only `ADMIN_USER_IDS` UUID allowlist. Configure `SUPABASE_URL` and `SUPABASE_PUBLISHABLE_KEY` (or legacy `SUPABASE_ANON_KEY`) on the API; never use a service-role key. Configure the browser with the public `NEXT_PUBLIC_SUPABASE_URL` and `NEXT_PUBLIC_SUPABASE_PUBLISHABLE_KEY` values.

An admin matches a request to an existing player through the protected player search or explicitly creates a player from the submitted details. The system never matches a public request to an existing player based on unverified contact information. Approval locks and rechecks the session, counts confirmed bookings, inserts the confirmed booking together with its `booking_participants` rows, and records the admin UUID and timestamp atomically. If capacity has filled or the session is no longer eligible, approval fails and the request remains pending for an explicit decision. Configure `ADMIN_DATABASE_URL` with its separate restricted login before enabling review.

Approved-request email notifications are supplemental to personal admin follow-up. The API creates an immutable notification snapshot in the same transaction as approval, then sends it after approval commits. Configure the backend-only `RESEND_API_KEY` and `RESEND_FROM_EMAIL` to enable email; both are optional, and approvals still succeed without them. Verify the sender address or its domain in the Resend dashboard before sending. A Resend acceptance response is not proof of delivery, so admins must continue direct follow-up. Failed or unconfigured sends can be retried only through the authenticated admin API, using the same stable Resend idempotency key. Do not put either setting in frontend environment variables or expose them to the browser.

For local admin password recovery, run the web app at `http://localhost:3001`, configure `NEXT_PUBLIC_SUPABASE_URL` and `NEXT_PUBLIC_SUPABASE_PUBLISHABLE_KEY` (or the legacy anon key) in the ignored `apps/web/.env.local`, and add `http://localhost:3001/admin/update-password` to the Supabase project's Auth **URL Configuration → Redirect URLs** allowlist. The recovery email redirects to that exact URL. This is a redirect allowlist setting only; admin authorization remains enforced by the Go API's server-side `ADMIN_USER_IDS` allowlist.

The public booking-request endpoint allows 5 requests per client IP per hour, and private booking-status lookups allow 30 per client IP per minute. Limits are stored in bounded, in-process memory and therefore apply independently on each API instance; they reset when an instance restarts and are not a substitute for shared abuse controls. For Vercel-to-Go API requests, configure the same random secret of at least 32 characters as server-only `API_PROXY_SHARED_SECRET` in both the Next.js deployment and Go API deployment. The Next.js proxy reads the platform-provided `x-vercel-forwarded-for` client IP and sends it to the Go API with that secret; the Go API accepts that IP only when the secret validates. Production booking proxy requests fail with 503 if either setting or the platform IP is missing or invalid. Direct Go API requests without a valid proxy secret are keyed by their connection `RemoteAddr`. Never expose the secret via a `NEXT_PUBLIC_` variable. Limits remain per API process, so keep HTTPS enabled and use gateway/shared abuse controls for multi-instance protection.

### Suggestions and feedback

The public `/suggestions` page submits a category, message, and optional name/email to `POST /api/feedback-submissions`. Submissions are stored through the `mmsu_booking_api` role and are never returned to the public. The `message` is required (1–2000 characters); `name` (≤200 characters) and `email` (a valid address, ≤254 bytes) are optional so anonymous submissions are supported. The endpoint allows 10 requests per client IP per hour using the same trusted-proxy identity and shared-secret rules as public booking requests.

Authorized admins review submissions in a protected inbox at `/admin/feedback`, which reads `GET /api/admin/feedback-submissions` and changes review status through `PATCH /api/admin/feedback-submissions/{id}` with `new`, `reviewed`, or `archived`. Both admin endpoints require the same `ADMIN_USER_IDS` verification as other admin routes and use the separate admin API role. The `/faq` page is a static reference of confirmed club membership dues, court and lighting fees, equipment rental, and booking guidance; it does not read from the database.

The Go server allows 5 seconds to receive request headers and 15 seconds to receive the complete request. Booking operations use a 5-second context deadline; Supabase Auth token validation uses a 2-second timeout and fails closed if Auth is unavailable.

## Future database changes

Add each schema change as a new, ordered SQL migration file under `supabase/migrations/`. Do not edit an already-applied migration to change a deployed schema; create a later migration instead. This makes the database history reviewable alongside the application code.
