# Next.js (SSR) and Go, sharing SQLite, on one EC2

The app is for a handful of friends, so it runs on a single small EC2 machine. Two servers sit behind Caddy, which provides automatic TLS:

- **Next.js** runs as a Node server with server-side rendering. It owns the web UI and the Discord login, using Better Auth.
- **Go** owns the domain logic, the API, the Discord bot (discordgo, gateway connection), and timed jobs.

Data lives in one **SQLite** file (`modernc.org/sqlite`, no cgo), which **Litestream** continuously replicates to S3. Each server runs as a systemd unit. Deploy is a script that copies the build over and restarts the units.

## Considered Options

- **One TypeScript process for web and bot**: rejected because we wanted Go for the backend.
- **Next.js static export served by Go**: rejected in favour of keeping Next.js as a full server, so it can own login through Better Auth.
- **A generated API contract (OpenAPI)**: rejected as too much ceremony for a friends-only app. The TypeScript and Go types are kept in sync by hand.

## Consequences

- Timed jobs (reminders, voting deadlines, marking an Esemény as Lezajlott) are stored as rows in SQLite and polled once a minute by the Go process. A restart therefore never loses one, and a job missed during downtime fires late rather than never.
- Two languages means two builds and two units to deploy, and the hand-kept API types can drift apart. That's the price of the trade-off.
