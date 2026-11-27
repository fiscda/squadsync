import "server-only";
import { headers } from "next/headers";
import { redirect } from "next/navigation";
import { cache } from "react";
import { auth } from "./auth";
import { type Felhasznalo, firstLogin, getFelhasznalo } from "./backend";

export type CurrentFelhasznalo =
  /** Nobody is logged in. */
  | { status: "anonymous" }
  /** Logged in, but Go doesn't know the Felhasználó yet. */
  | { status: "missing" }
  | { status: "ok"; felhasznalo: Felhasznalo };

/**
 * Reads the logged-in Felhasználó as Go knows them. Read-only, so it's safe
 * to call while rendering, and cached for the duration of one request.
 */
export const getCurrentFelhasznalo = cache(
  async (): Promise<CurrentFelhasznalo> => {
    const session = await auth.api.getSession({ headers: await headers() });
    if (!session) {
      return { status: "anonymous" };
    }
    const felhasznalo = await getFelhasznalo(session.user.id);
    return felhasznalo ? { status: "ok", felhasznalo } : { status: "missing" };
  },
);

/**
 * Makes sure Go knows the logged-in Felhasználó, repeating the idempotent
 * first-login call if needed. Go normally learns about them from the Better
 * Auth hook; this covers a hook call that failed (for example, Go was down).
 *
 * It's a mutation, so call it only from a route handler or a server action,
 * never while rendering. Returns the Felhasználó's id, or null when nobody is
 * logged in.
 */
export async function ensureFelhasznalo(): Promise<string | null> {
  const requestHeaders = await headers();
  const session = await auth.api.getSession({ headers: requestHeaders });
  if (!session) {
    return null;
  }
  if (await getFelhasznalo(session.user.id)) {
    return session.user.id;
  }

  const accounts = await auth.api.listUserAccounts({
    headers: requestHeaders,
  });
  const discord = accounts.find((a) => a.providerId === "discord");
  if (!discord) {
    throw new Error(`user ${session.user.id} has no Discord account`);
  }
  await firstLogin(session.user.id, {
    discordId: discord.accountId,
    name: session.user.name,
  });
  return session.user.id;
}

/**
 * The guard for pages that need a login: the logged-in Felhasználó, or a
 * redirect to the home page, which offers the login (or the retry when Go
 * doesn't know them yet). Read-only, so it's safe while rendering.
 */
export async function requireFelhasznalo(): Promise<Felhasznalo> {
  const current = await getCurrentFelhasznalo();
  if (current.status !== "ok") {
    redirect("/");
  }
  return current.felhasznalo;
}

/**
 * Starts the Discord login and returns the Discord URL to send the browser
 * to. After Discord, Better Auth redirects to callbackURL, which must be a
 * path on this site.
 */
export async function discordLoginUrl(callbackURL: string): Promise<string> {
  const { url } = await auth.api.signInSocial({
    body: { provider: "discord", callbackURL },
    headers: await headers(),
  });
  if (!url) {
    throw new Error("Better Auth returned no Discord authorization URL");
  }
  return url;
}
