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
4. Under **Redirects**, add both of these and save:
   - `http://localhost:3000/api/auth/callback/discord` (web login, Better Auth)
   - `http://localhost:3000/fiok/discord/callback` (linking a further Discord account on `/fiok`)
   - In production, add the same two paths on `https://<your domain>` too. Both are built from `BETTER_AUTH_URL`.

Login asks only for the `identify` and `email` scopes; linking asks only for `identify`. Discord accounts registered with only a phone number have no email, so they get a placeholder address on the reserved `.invalid` domain. The app never sends email.

### 1b. Add the bot (optional)

Go runs without the bot when `DISCORD_BOT_TOKEN` is empty, so you can skip this until you work on Discord features.

The bot connects in the background. If Discord can't be reached when Go starts (an outage, no network, a wrong token), Go logs the error and keeps retrying with backoff, from 5 seconds up to every 5 minutes, while the internal API keeps working. Once connected, it reconnects by itself.

1. In the same application, open **Bot** in the sidebar. Click **Reset Token** and copy the token into `DISCORD_BOT_TOKEN`.
   - No privileged gateway intents are needed: leave **Presence**, **Server Members** and **Message Content** off.
   - Optionally, turn off **Public Bot** so only you can add it to servers.
2. Open **OAuth2** → **OAuth2 URL Generator**. Tick the scopes `bot` and `applications.commands`, then the bot permissions **View Channels**, **Send Messages**, **Embed Links** and **Read Message History**. Open the generated URL and add the bot to your test server. The invite URL has this shape:
   `https://discord.com/oauth2/authorize?client_id=<CLIENT_ID>&scope=bot+applications.commands&permissions=84992`
3. For instant slash-command updates, set `DISCORD_DEV_GUILD_ID` to your test server's id: in Discord, turn on **User Settings** → **Advanced** → **Developer Mode**, right-click the server icon and click **Copy Server ID**. Without it, commands are registered globally.

In a channel of that server, an Admin of a Csoport runs `/csatorna` and picks the Csoport to link the channel to it. The Csoport page on the web shows the linked channel, and Admins can unlink it there.

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
| `DISCORD_CLIENT_ID`, `DISCORD_CLIENT_SECRET` | The values from step 1. Go uses them too, to exchange the code when a Felhasználó links a further Discord account. |
| `DISCORD_BOT_TOKEN`, `DISCORD_DEV_GUILD_ID` | Optional: the values from step 1b. |

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

- **Go:** the server also migrates on every start. Applied versions are recorded in `schema_migrations`. To add a migration:
  - Name the file `YYYYMMDDHHMM_description.sql`, using the current UTC time (`date -u +%Y%m%d%H%M`). Timestamps keep parallel branches from picking the same number.
  - Two files with the same version make the migration fail.
  - Never edit or rename a file once it has been applied anywhere. A version recorded under a different file name makes the migration fail.
  - Each file runs in one transaction with foreign keys enforced. SQLite can't turn them off inside a transaction, so a migration that rebuilds a table (create new, copy, drop old, rename) must start with `PRAGMA defer_foreign_keys = ON;`. The checks then run at commit.
- **Better Auth:** the CLI reads `squadsync-fe/src/lib/auth.ts` and adds missing tables and columns.

## Checks

```sh
make test   # cd squadsync-be && go test ./...; cd squadsync-fe && bun test
make lint   # cd squadsync-fe && bun run lint && bunx tsc --noEmit
```

`bunx tsc --noEmit` needs Next.js's generated route types (`LayoutProps` and others). `next dev` and `next build` create them, and `make lint` runs `next typegen` first.

## How the pieces talk

1. The browser talks only to Next.js. Every call to Go happens on the server, in a server component, server action or route handler, through `squadsync-fe/src/lib/backend.ts`.
2. Each call to Go sends two headers:
   - `X-Internal-Secret`: the shared secret. Go answers `401` without it.
   - `X-Felhasznalo-Id`: Better Auth's `user.id`, which is also the Felhasználó id.
3. **First login.** When Better Auth creates the Discord account of a new user, a database hook (`squadsync-fe/src/lib/auth.ts`) calls `POST /felhasznalo/first-login` with the Discord user id. Go creates the Felhasználó and records the main Discord account. The call is idempotent.
   - If that call failed (for example, Go was down), it's repeated by `ensureFelhasznalo` (`squadsync-fe/src/lib/felhasznalo.ts`). This runs in the `/login/complete` route handler, where Better Auth redirects after every login. If the Felhasználó is still missing, the page offers a retry button, which calls a server action.
   - Rendering never mutates. Pages only read, through `getCurrentFelhasznalo`.
4. **Linking.** Better Auth's own account linking is turned off. Linking more Discord accounts belongs to Go (ADR-0002):
   - On `/fiok`, the "Másik Discord fiók kapcsolása" button posts to `/fiok/discord/kapcsolas`. That route stores a random `state` in an httpOnly cookie bound to the Felhasználó, and sends the browser to Discord (scope `identify`, `prompt=consent`, so you can switch to the other account with "Not you?").
   - Discord redirects to `/fiok/discord/callback`. The route checks the state, then forwards the code and the redirect URI to `POST /felhasznalo/discord-account`. Go exchanges the code with Discord (`gateway.DiscordOAuth`) and records the link.
   - An account of a Felhasználó with a Tag or any other data is refused ("Ez a fiók már használatban van."). One whose Felhasználó has nothing else (they only logged in once) is moved over, that empty Felhasználó is removed, and Next.js ends its Better Auth sessions.
   - Extra accounts can be unlinked on `/fiok`; the main one can't.
5. **Web login with a linked account is refused.** Better Auth's `user.validateUserInfo` (`squadsync-fe/src/lib/web-login-guard.ts`) asks Go's secret-only `GET /discord-account/{discordId}/web-login`. It runs before Better Auth creates a user and before it creates the session of an existing one, so a linked account gets neither. The refusal ("Ez a fiók X-hez van kapcsolva, lépj be azzal.") is shown on the home page.
6. **DMs** to a Felhasználó go to every linked account: use `App.SendDMToFelhasznalo`.

## Go backend layout

| Package | Contents |
| --- | --- |
| `cmd/squadsync` | `main`. It reads the environment, migrates and serves. `squadsync migrate` only migrates. |
| `internal/app` | **The application core.** Every use case is a method on `app.App`, which is the test seam. Generic errors live in `app.go`; area-specific errors live in that area's file. |
| `internal/domain` | Plain domain types (`Felhasznalo`, `DiscordAccount`). |
| `internal/store` | SQLite: open (WAL, busy timeout, `BEGIN IMMEDIATE`), embedded migrations (`migrations/`) and queries. Use `Store.InTx` for transactions. |
| `internal/httpapi` | The thin internal HTTP API. A global middleware checks the secret, and `withFelhasznalo` requires the Felhasználó id on routes that act for one. Each area registers its routes in its own file with `register<Area>(mux)`. |
| `internal/bot` | The Discord bot: a thin adapter like `httpapi`. Its `Router` resolves every interaction to a Felhasználó, then routes slash commands by name and buttons and modals by custom-id prefix (`name:arg:arg`). Handlers that may exceed Discord's 3-second limit return a deferred reply. See the package comment for how to add a handler. `bot.Messenger` is the real `gateway.DiscordMessenger`. |
| `internal/gateway` | Outbound interfaces: Discord messaging, Discord OAuth (for linking, with the real `DiscordOAuthClient`), Google Calendar and Spotify. |
| `internal/gateway/gatewayfake` | Recording fakes of the gateways, for tests. |
| `internal/clock` | The `Clock` interface, `clock.System` and the controllable `clock.Fake`. |
| `internal/apptest` | The test harness. `apptest.New(t)` gives an `App` on a temporary SQLite file with the real migrations, a fake clock and the recording fakes. |

Core tests live next to the use cases (for example `internal/app/felhasznalo_test.go`), in package `app_test`, and use `apptest.New(t)`.
