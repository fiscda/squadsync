// The Better Auth gate that refuses web login with a linked Discord account
// (ADR-0002). Only a Felhasználó's main account logs in to the web; Go knows
// which accounts are linked ones.
//
// Kept free of Better Auth and Next.js imports so it can be tested alone
// (web-login-guard.test.ts). Also loaded by the Better Auth CLI through
// auth.ts, so it uses relative imports only.
import type { WebLoginCheck } from "./backend";

/** The error code in the login error redirect (`?error=...`). */
export const LINKED_ACCOUNT_ERROR = "discord_account_linked";
/** When Go couldn't be asked: the login is refused rather than let through. */
export const LOGIN_CHECK_FAILED_ERROR = "login_check_failed";

/** The part of Better Auth's validateUserInfo input that the gate reads. */
export type LoginSource = {
  /** "create-user", "sign-in" or "link-account". */
  action: string;
  method: string;
  oauth?: { providerId: string; profile?: Record<string, unknown> };
};

export type LoginRefusal = { error: string; errorDescription: string };

/**
 * Decides a web login for Better Auth's `user.validateUserInfo`. It runs
 * before Better Auth creates a new user ("create-user") and, for a returning
 * user, before it creates the session ("sign-in"), so a refused account gets
 * neither. Returns the refusal, or undefined to let the login through.
 */
export async function refuseLinkedDiscordAccount(
  source: LoginSource,
  check: (discordId: string) => Promise<WebLoginCheck>,
): Promise<LoginRefusal | undefined> {
  if (source.method !== "oauth" || source.oauth?.providerId !== "discord") {
    return undefined;
  }
  // Better Auth's own account linking is off (auth.ts); Go owns linking.
  if (source.action !== "create-user" && source.action !== "sign-in") {
    return undefined;
  }

  const discordId = source.oauth.profile?.id;
  if (typeof discordId !== "string" || discordId === "") {
    return {
      error: LOGIN_CHECK_FAILED_ERROR,
      errorDescription: "A Discord nem küldte el a fiókod azonosítóját.",
    };
  }

  let result: WebLoginCheck;
  try {
    result = await check(discordId);
  } catch (err) {
    console.error("web login check: Go call failed", err);
    return {
      error: LOGIN_CHECK_FAILED_ERROR,
      errorDescription: "Most nem tudunk beléptetni. Próbáld újra később.",
    };
  }
  if (result.allowed) {
    return undefined;
  }
  return { error: LINKED_ACCOUNT_ERROR, errorDescription: result.message };
}
