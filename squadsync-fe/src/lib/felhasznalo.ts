import "server-only";
import { headers } from "next/headers";
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
 * never while rendering. Returns false when nobody is logged in.
 */
export async function ensureFelhasznalo(): Promise<boolean> {
  const requestHeaders = await headers();
  const session = await auth.api.getSession({ headers: requestHeaders });
  if (!session) {
    return false;
  }
  if (await getFelhasznalo(session.user.id)) {
    return true;
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
  return true;
}
