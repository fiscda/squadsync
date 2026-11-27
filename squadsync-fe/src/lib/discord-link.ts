import "server-only";
import { randomBytes, timingSafeEqual } from "node:crypto";
import { cache } from "react";
import { type DiscordAccount, listDiscordAccounts } from "./backend";
import { requireFelhasznalo } from "./felhasznalo";

// Linking a further Discord account (ADR-0002). It is a Discord OAuth flow of
// its own, separate from Better Auth's login:
//
//   1. POST /fiok/discord/kapcsolas stores a random state in an httpOnly
//      cookie, bound to the logged-in Felhasználó, and sends the browser to
//      Discord's authorize page.
//   2. Discord sends the browser back to /fiok/discord/callback with a code
//      and the state. The route checks the state against the cookie and
//      forwards the code and the redirect URI to Go.
//   3. Go exchanges the code with Discord and records the link.
//
// The redirect URI below must be registered in the Discord application
// (see README.md).

/** The cookie that holds the state of a linking flow in progress. */
export const LINK_STATE_COOKIE = "squadsync_discord_link";
/** The flow must finish within this many seconds. */
export const LINK_STATE_MAX_AGE = 10 * 60;
/** The cookie is sent only to the linking routes. */
export const LINK_STATE_PATH = "/fiok/discord";

/** Where Discord sends the browser back after the linking flow. */
export function linkRedirectUri(): string {
  return `${publicUrl()}/fiok/discord/callback`;
}

/** Whether the site is served over HTTPS, so cookies can be Secure. */
export function isHttps(): boolean {
  return publicUrl().startsWith("https://");
}

function publicUrl(): string {
  const url = process.env.BETTER_AUTH_URL?.replace(/\/+$/, "");
  if (!url) {
    throw new Error("BETTER_AUTH_URL is not set (see .env.example)");
  }
  return url;
}

/** The contents of the state cookie: the state and whom it belongs to. */
export type LinkState = { state: string; felhasznaloId: string };

export function newLinkState(felhasznaloId: string): LinkState {
  return { state: randomBytes(32).toString("base64url"), felhasznaloId };
}

export function encodeLinkState(s: LinkState): string {
  return JSON.stringify(s);
}

/**
 * Whether the state Discord sent back matches the cookie, and the cookie
 * belongs to the logged-in Felhasználó. Anything malformed is a mismatch.
 */
export function linkStateMatches(
  cookieValue: string | undefined,
  returnedState: string | null,
  felhasznaloId: string,
): boolean {
  if (!cookieValue || !returnedState) {
    return false;
  }
  let stored: Partial<LinkState>;
  try {
    stored = JSON.parse(cookieValue);
  } catch {
    return false;
  }
  if (
    typeof stored.state !== "string" ||
    stored.felhasznaloId !== felhasznaloId
  ) {
    return false;
  }
  const a = Buffer.from(stored.state);
  const b = Buffer.from(returnedState);
  return a.length === b.length && timingSafeEqual(a, b);
}

/** Discord's authorize page for linking, asking only for `identify`. */
export function discordAuthorizeUrl(state: string): string {
  const clientId = process.env.DISCORD_CLIENT_ID;
  if (!clientId) {
    throw new Error("DISCORD_CLIENT_ID is not set (see .env.example)");
  }
  const params = new URLSearchParams({
    response_type: "code",
    client_id: clientId,
    scope: "identify",
    redirect_uri: linkRedirectUri(),
    state,
    // Always show Discord's consent screen. Otherwise Discord approves at
    // once for the account the browser is logged in to (usually the main
    // one), leaving no chance to switch to the other account.
    prompt: "consent",
  });
  return `https://discord.com/oauth2/authorize?${params}`;
}

/** The logged-in Felhasználó's Discord accounts, the main one first. */
export const getMyDiscordAccounts = cache(
  async (): Promise<DiscordAccount[]> => {
    const felhasznalo = await requireFelhasznalo();
    return listDiscordAccounts(felhasznalo.id);
  },
);
