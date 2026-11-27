# Next.js is the only gateway, and Go stays on localhost

Login lives in Next.js (Better Auth), and Better Auth's session cookie is signed and opaque, so Go can't validate it alone. Rather than have Go check sessions, we made **Next.js the only public surface**. Caddy routes all traffic to Next.js. Next.js validates the session in-process and calls Go on localhost, passing the Felhasználó id in a header guarded by a shared secret. Go has no public endpoint at all. Its Discord bot connects outward to Discord.

## Considered Options

- **Browser calls Go directly, and Go calls Better Auth's `get-session`**: rejected. It adds an extra round trip on every request and exposes a second public service.
- **Better Auth JWT plugin, verified in Go with JWKS**: rejected. It means key handling and delayed revocation for no benefit at this scale.

## Consequences

- **Table ownership in the shared SQLite file.** Better Auth owns `user`, `session`, `account` and `verification`, and migrates them with its own CLI. Go owns every domain table and has its own migrations. **Go never reads or writes Better Auth's tables.**
- **`user.id` is the Felhasználó id.** Next.js tells Go about each new user after their first login. The bot resolves a Discord user to a Felhasználó only through Go's own Discord-account table (ADR-0002).
- **Every web feature needs a pass-through in Next.js.** Go's API is internal, so each web feature needs a Next.js server action or route handler that forwards to Go.
