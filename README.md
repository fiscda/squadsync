# squadsync

Squad Sync is a Hungarian-language web app and Discord bot that helps a small friend group find a time to meet and organise the gathering. Read `GLOSSARY.md` for the domain language and `docs/adr/` for the architecture decisions.

- `squadsync-fe/`: Next.js. It's the only public surface, and it handles Discord login through Better Auth.
- `squadsync-be/`: Go. It holds the domain logic and an internal HTTP API that listens on `127.0.0.1` only.
- Both use one SQLite file in WAL mode. Better Auth owns the `user`, `session`, `account` and `verification` tables. Go owns every other table and never touches Better Auth's.

## Prerequisites

- **Go:** `go.mod` asks for Go 1.26.4. With an older local Go, `GOTOOLCHAIN=auto` (the default) downloads it automatically.
- **Bun 1.3.9.** Always use `bun` for the frontend, never npm, yarn or pnpm.
- **Node.js 24** (22.13 or later also works). Next.js runs on Node, and Better Auth uses Node's built-in `node:sqlite`, so there's no native module to build.
- **make** and **openssl** (or any other way to generate random secrets).

## Setup

### 1. Create a Discord application for login

Every developer uses their own test application.

1. Open the [Discord Developer Portal](https://discord.com/developers/applications) and click **New Application**. Any name works, for example "Squad Sync (dev)".
2. Open **OAuth2** in the sidebar.
3. Copy the **Client ID**. Then click **Reset Secret** and copy the **Client Secret**.
4. Under **Redirects**, add `http://localhost:3000/api/auth/callback/discord` and save.
   - In production, add `https://<your domain>/api/auth/callback/discord` too.

Login asks only for the `identify` and `email` scopes, and no bot is needed yet.

### 2. Configure the environment

```sh
cp .env.example .env
```

Fill in `.env`:

| Variable | What to put there |
| --- | --- |
| `DATABASE_PATH` | The SQLite file both servers share. The default is `./data/squadsync.db`. |
| `INTERNAL_API_SECRET` | The output of `openssl rand -hex 32`. Next.js sends it to Go on every call. |
| `BACKEND_PORT` | The port Go listens on, on `127.0.0.1`. The default is `8081`. |
| `BETTER_AUTH_SECRET` | The output of `openssl rand -base64 32`. |
| `BETTER_AUTH_URL` | `http://localhost:3000` |
| `DISCORD_CLIENT_ID`, `DISCORD_CLIENT_SECRET` | The values from step 1. |

The Makefile exports everything in `.env` to both servers, so there's no separate `.env` inside `squadsync-fe/` or `squadsync-be/`.

### 3. Install, migrate and run

```sh
make install   # frontend dependencies
make dev       # migrates, then starts Go and Next.js; Ctrl+C stops both
```

Open http://localhost:3000 and click **Belépés Discorddal**.

## Migrations

Each side migrates only its own tables in the shared SQLite file. `make dev` runs both migrations first. To run them on their own:

```sh
make migrate       # both
make migrate-be    # Go: embedded SQL files in squadsync-be/internal/store/migrations/
make migrate-fe    # Better Auth CLI: bun run auth:migrate (in squadsync-fe/)
```

- **Go:** the server also migrates on every start. Add a new file `NNNN_description.sql` to the migrations folder, and never edit one that has already been applied. Applied versions are recorded in `schema_migrations`.
- **Better Auth:** the CLI reads `squadsync-fe/src/lib/auth.ts` and adds missing tables and columns.

## Checks

```sh
make test   # cd squadsync-be && go test ./...
make lint   # cd squadsync-fe && bun run lint && bunx tsc --noEmit
```

`bunx tsc --noEmit` needs Next.js's generated route types (`LayoutProps` and others). `next dev` and `next build` create them, and `make lint` runs `next typegen` first.

## How the pieces talk

1. The browser talks only to Next.js. Every call to Go happens on the server, in a server component, server action or route handler, through `squadsync-fe/src/lib/backend.ts`.
2. Each call to Go sends two headers:
   - `X-Internal-Secret`: the shared secret. Go answers `401` without it.
   - `X-Felhasznalo-Id`: Better Auth's `user.id`, which is also the Felhasználó id.
3. **First login.** When Better Auth creates the Discord account of a new user, a database hook (`squadsync-fe/src/lib/auth.ts`) calls `POST /felhasznalo/first-login` with the Discord user id. Go creates the Felhasználó and records the main Discord account. The call is idempotent.
   - If Go was down at that moment, the page repeats the call the next time it can't find the Felhasználó (`squadsync-fe/src/lib/felhasznalo.ts`).

## Go backend layout

| Package | Contents |
| --- | --- |
| `cmd/squadsync` | `main`. It reads the environment, migrates and serves. `squadsync migrate` only migrates. |
| `internal/app` | **The application core.** Every use case is a method on `app.App`, which is the test seam. |
| `internal/domain` | Plain domain types (`Felhasznalo`, `DiscordAccount`). |
| `internal/store` | SQLite: open (WAL, busy timeout, `BEGIN IMMEDIATE`), embedded migrations and queries. Use `Store.InTx` for transactions. |
| `internal/httpapi` | The thin internal HTTP API. It checks the secret, translates requests and calls one use case. |
| `internal/gateway` | Outbound interfaces: Discord messaging, Google Calendar and Spotify. |
| `internal/gateway/gatewayfake` | Recording fakes of the gateways, for tests. |
| `internal/clock` | The `Clock` interface, `clock.System` and the controllable `clock.Fake`. |
| `internal/apptest` | The test harness. `apptest.New(t)` gives an `App` on a temporary SQLite file with the real migrations, a fake clock and the recording fakes. |

Core tests live next to the use cases (for example `internal/app/felhasznalo_test.go`), in package `app_test`, and use `apptest.New(t)`.

## Known limitations

- Discord returns no email for phone-only Discord accounts, and Better Auth then refuses the login (`email_not_found`).
