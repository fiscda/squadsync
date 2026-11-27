import "server-only";
import { cache } from "react";
import { checkWebLogin } from "./backend";
import {
  LINKED_ACCOUNT_ERROR,
  LOGIN_CHECK_FAILED_ERROR,
  verifySignedDiscordId,
} from "./web-login-guard";

// A refused login comes back to the home page with `?error=<code>` and
// `&error_description=`. Both are in the URL, so anyone can forge them: no
// text from the URL is ever shown. Each code maps to fixed text, and the
// linked-account refusal re-asks Go for its message (it names the main
// account), from the signed Discord id in error_description.

const fixedMessages: Record<string, string> = {
  [LOGIN_CHECK_FAILED_ERROR]:
    "Most nem tudunk beléptetni. Próbáld újra később.",
};
const genericMessage = "A belépés nem sikerült. Próbáld újra.";

/** Go's web-login check, cached for one request. Read-only. */
const getWebLoginCheck = cache(checkWebLogin);

/** The text to show for a refused login, or null when there's no error. */
export async function loginErrorMessage(
  error: string | undefined,
  description: string | undefined,
): Promise<string | null> {
  if (!error) {
    return null;
  }
  if (error !== LINKED_ACCOUNT_ERROR) {
    return fixedMessages[error] ?? genericMessage;
  }
  const discordId = description
    ? verifySignedDiscordId(description, process.env.BETTER_AUTH_SECRET ?? "")
    : null;
  if (!discordId) {
    return genericMessage;
  }
  const check = await getWebLoginCheck(discordId);
  // Allowed by now (say, unlinked meanwhile): nothing specific to say.
  return check.allowed ? genericMessage : check.message;
}
