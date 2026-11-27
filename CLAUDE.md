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

- Backend tests: `cd squadsync-be && go test ./...`
- Frontend lint: `cd squadsync-fe && bun run lint`
- Frontend type check: `cd squadsync-fe && bunx tsc --noEmit`
- Frontend dependencies: always use `bun` (never npm, yarn or pnpm).

## Rules for implementation agents

- **Branches:** work only on your ticket branch. Commit there, push it, and open a PR that closes the ticket. Never commit to `main`, never merge, and never push to another branch.
- **Next.js:** this Next.js version differs from your training data. Read the relevant guide in `squadsync-fe/node_modules/next/dist/docs/` before writing frontend code, and heed deprecation notices.
- **Naming:** use the domain terms from `GLOSSARY.md`. In Go and TypeScript identifiers, drop the accents (`Esemény` → `Esemeny`) and keep everything else in English (ADR-0001). All UI text is in Hungarian.
- **Testing:** test behaviour through the Go application core (use cases), using a real temporary SQLite file, the controllable clock and the recording fakes for Discord, Google and Spotify. Don't test private helpers.
- **Before opening a PR:** all the commands above must pass locally.
- **No CI:** don't add CI or GitHub Actions workflows.
- **Real credentials:** never call real Discord, Google or Spotify APIs in tests, and never commit secrets. Document required environment variables in a `.env.example`.
- **Docker:** if it's ever needed, don't use port 5432, and always use persisted volumes.
