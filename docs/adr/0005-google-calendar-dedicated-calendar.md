# Google Calendar sync uses a dedicated calendar and an unverified app

Felhasználók can connect Google Calendar, separately from login, which stays Discord-only. Confirmed Esemény then appear in their calendar for as long as they are a Résztvevő. We request only the `calendar.app.created` scope. The app creates its own "Squad Sync" calendar in each person's Google account, never touches their existing calendars, and can be hidden in one click.

The Google OAuth app is **published but not verified**. Google's limits make this the only workable option for us:
- **Testing mode** expires refresh tokens after 7 days, so everyone would have to reconnect weekly.
- **Verification** is overkill for friends.
- **Unverified production** allows 100 users over the app's lifetime, and each user sees a one-time warning screen.

## Considered Options

- **An iCal feed each person subscribes to**: rejected. Google refreshes subscribed feeds on its own schedule, so changes arrive late.
- **The `calendar.events` scope, writing into the primary calendar**: rejected. It's broader access than we need, and events would be mixed in with the person's own.

## Consequences

- **Go owns the Google tokens** in its own table. Go has no public endpoint (ADR-0004), so a Next.js route receives the OAuth callback and hands the code to Go.
- **The 100-user lifetime cap can't be reset.** If this ever becomes a product, the Google app needs verification.
