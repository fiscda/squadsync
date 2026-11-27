# squadsync

## Agent skills

### Issue tracker

Issues and specs are tracked as GitHub Issues on `fiscda/squadsync`, using the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

The five default labels: `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: `GLOSSARY.md` and `docs/adr/` at the repo root. Domain terms are Hungarian. See `docs/agents/domain.md`.

## Repository layout

- `squadsync-be/`: Go backend (module `squadsync-be`). Holds the domain core, the internal HTTP API (localhost only), the Discord bot and timed jobs.
- `squadsync-fe/`: Next.js frontend (bun, Biome, Tailwind 4). It's the only public surface and handles login through Better Auth.
- `GLOSSARY.md` and `docs/adr/`: the domain language and architectural decisions. Read them before working on a ticket.

## Commands

- **Every check:** `make test` (Go tests) and `make lint` (Biome, Next typegen, tsc). Both must pass before you open a PR. Also run `cd squadsync-be && go vet ./... && gofmt -l .`.
- **Running locally:** `make install`, then `make dev`. Variables come from the root `.env`, and `.env.example` lists them all.
- **Frontend dependencies:** always use `bun` (never npm, yarn or pnpm).

## Code conventions (established in #2, follow them)

**Backend (`squadsync-be/internal/`)**
- **Use cases:** methods on `*app.App` in `app/<area>.go`. Area-specific errors live in that area's file. Only `ErrNotFound` and `ErrInvalidInput` live in `app.go`.
- **Tests:** in package `app_test`, using `apptest.New(t)`, which provides the app, the store, a fake clock and recording fakes. Test behaviour through use cases.
- **Migrations:** `store/migrations/YYYYMMDDHHMM_description.sql`. **Use the exact timestamp the coordinator gives you in your task prompt.** Parallel tickets created in the same minute collided once. If you weren't given one, use the current UTC time and check that it doesn't exist on `main`. If you need several migrations, add one minute for each. Never edit a migration that has been merged; add a new one instead. A migration that rebuilds a table uses `PRAGMA defer_foreign_keys = ON`.
- **Store:** query methods on `store.Queries`. Use `Store.InTx` for anything that writes several rows.
- **HTTP API:** each area registers its routes in its own file through `register<Area>(mux)`, and maps its own errors to status codes there. Wrap routes that need the caller with `withFelhasznalo(handler)`. Add `httptest` tests for new routes.
- **Gateways:** outbound integrations go behind interfaces in `gateway/`, with recording fakes in `gateway/gatewayfake`. Extend those; never call real APIs in tests.
- **Off limits:** never touch Better Auth's tables (`user`, `session`, `account`, `verification`).

**Frontend (`squadsync-fe/src/`)**
- **Calling Go:** only from server code, through `lib/backend.ts`. Keep its types in sync with Go by hand.
- **Reads and writes:** reads are side-effect free and wrapped in React `cache()`. Writes happen only in server actions or route handlers, never while rendering.
- **Next.js 16:** check `squadsync-fe/node_modules/next/dist/docs/` for anything to do with `cacheComponents`, Suspense or route handlers. In route handlers, `unstable_rethrow` is needed inside `catch` blocks.

## Rules for implementation agents

- **Branches:** work only on your ticket branch. Commit there, push it, and open a PR that closes the ticket. Never commit to `main`, never merge, and never push to another branch.
- **Next.js:** this Next.js version differs from your training data. Read the relevant guide in `squadsync-fe/node_modules/next/dist/docs/` before writing frontend code, and heed deprecation notices.
- **Naming:** use the domain terms from `GLOSSARY.md`. In Go and TypeScript identifiers, drop the accents (`Esemény` → `Esemeny`) and keep everything else in English (ADR-0001). All UI text is in Hungarian.
- **Testing:** test behaviour through the Go application core (use cases), using a real temporary SQLite file, the controllable clock and the recording fakes for Discord, Google and Spotify. Don't test private helpers.
- **Before opening a PR:** all the commands above must pass locally.
- **No CI:** don't add CI or GitHub Actions workflows.
- **Real credentials:** never call real Discord, Google or Spotify APIs in tests, and never commit secrets. Document required environment variables in a `.env.example`.
- **Docker:** if it's ever needed, don't use port 5432, and always use persisted volumes.
