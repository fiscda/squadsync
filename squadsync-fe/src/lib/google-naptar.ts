import "server-only";
import { cache } from "react";
import { type GoogleNaptar, getGoogleNaptar } from "./backend";
import { publicUrl } from "./discord-link";
import { requireFelhasznalo } from "./felhasznalo";

// Connecting Google Calendar (ADR-0005). The same shape as linking a Discord
// account (lib/discord-link.ts), with its own state cookie:
//
//   1. POST /fiok/google/kapcsolas stores a random state in an httpOnly
//      cookie, bound to the logged-in Felhasználó, and sends the browser to
//      Google's consent page.
//   2. Google sends the browser back to /fiok/google/callback with a code and
//      the state. The route checks the state against the cookie and forwards
//      the code to Go.
//   3. Go exchanges the code with Google, with the same redirect URI built
//      from its own BETTER_AUTH_URL, stores the refresh token and syncs.
//
// The redirect URI below must be registered in the Google OAuth client
// (see README.md).

/** The cookie that holds the state of a connecting flow in progress. */
export const GOOGLE_STATE_COOKIE = "squadsync_google_link";
/** The cookie is sent only to the connecting routes. */
export const GOOGLE_STATE_PATH = "/fiok/google";

/**
 * The only scope asked for: make secondary calendars and manage the events
 * in them. The app never sees the Felhasználó's other calendars.
 */
export const GOOGLE_CALENDAR_SCOPE =
  "https://www.googleapis.com/auth/calendar.app.created";

/**
 * Where Google sends the browser back. Go builds the same URI for the code
 * exchange (app.GoogleNaptarRedirectURI).
 */
export function googleRedirectUri(): string {
  return `${publicUrl()}/fiok/google/callback`;
}

/**
 * Google's consent page, or null when the Google OAuth client isn't
 * configured. access_type=offline with prompt=consent makes Google return a
 * refresh token every time, also when connecting again.
 */
export function googleAuthorizeUrl(state: string): string | null {
  const clientId = process.env.GOOGLE_CLIENT_ID;
  if (!clientId) {
    return null;
  }
  const params = new URLSearchParams({
    response_type: "code",
    client_id: clientId,
    scope: GOOGLE_CALENDAR_SCOPE,
    redirect_uri: googleRedirectUri(),
    state,
    access_type: "offline",
    prompt: "consent",
  });
  return `https://accounts.google.com/o/oauth2/v2/auth?${params}`;
}

/** The logged-in Felhasználó's Google Calendar connection. */
export const getMyGoogleNaptar = cache(async (): Promise<GoogleNaptar> => {
  const felhasznalo = await requireFelhasznalo();
  return getGoogleNaptar(felhasznalo.id);
});
