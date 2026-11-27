import "server-only";
import { headers } from "next/headers";
import { auth } from "./auth";
import { type Felhasznalo, firstLogin, getFelhasznalo } from "./backend";

/**
 * Returns the logged-in Felhasználó as Go knows them, or null when nobody is
 * logged in.
 *
 * Go normally learns about a Felhasználó from the Better Auth hook on their
 * first login. If that call failed (say, Go was down), this repeats it: the
 * first-login call is idempotent.
 */
export async function getCurrentFelhasznalo(): Promise<Felhasznalo | null> {
  const requestHeaders = await headers();
  const session = await auth.api.getSession({ headers: requestHeaders });
  if (!session) {
    return null;
  }

  const felhasznalo = await getFelhasznalo(session.user.id);
  if (felhasznalo) {
    return felhasznalo;
  }

  const accounts = await auth.api.listUserAccounts({
    headers: requestHeaders,
  });
  const discord = accounts.find((a) => a.providerId === "discord");
  if (!discord) {
    throw new Error(`user ${session.user.id} has no Discord account`);
  }
  return firstLogin(session.user.id, {
    discordId: discord.accountId,
    name: session.user.name,
  });
}
