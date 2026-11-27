import { cookies } from "next/headers";
import { redirect, unstable_rethrow } from "next/navigation";
import { auth } from "@/lib/auth";
import { linkDiscordAccount } from "@/lib/backend";
import {
  LINK_STATE_COOKIE,
  LINK_STATE_PATH,
  linkStateMatches,
} from "@/lib/discord-link";
import { getCurrentFelhasznalo } from "@/lib/felhasznalo";

/** The outcomes /fiok explains, passed as `?kapcsolas=`. */
type KapcsolasResult =
  | "ok"
  | "mar-a-tied"
  | "foglalt"
  | "megszakitva"
  | "nem-elerheto"
  | "hiba";

// Discord sends the browser back here at the end of linking a further
// Discord account (see lib/discord-link.ts). The state must match the
// httpOnly cookie set for this Felhasználó when the flow started, which
// stops a forged callback from linking someone else's account to them.
export async function GET(request: Request) {
  const params = new URL(request.url).searchParams;
  const cookieStore = await cookies();
  const stored = cookieStore.get(LINK_STATE_COOKIE)?.value;
  // One use only, whatever happens next.
  cookieStore.delete({ name: LINK_STATE_COOKIE, path: LINK_STATE_PATH });

  const current = await getCurrentFelhasznalo();
  if (current.status !== "ok") {
    redirect("/");
  }
  const { felhasznalo } = current;

  let result: KapcsolasResult;
  try {
    result = await finishLinking(felhasznalo.id, stored, params);
  } catch (err) {
    unstable_rethrow(err); // let Next.js handle its own request-time signals
    console.error("linking a Discord account failed", err);
    result = "hiba";
  }
  redirect(`/fiok?kapcsolas=${result}`);
}

async function finishLinking(
  felhasznaloId: string,
  stored: string | undefined,
  params: URLSearchParams,
): Promise<KapcsolasResult> {
  if (!linkStateMatches(stored, params.get("state"), felhasznaloId)) {
    return "hiba";
  }
  // The user pressed Cancel on Discord's page.
  if (params.get("error")) {
    return "megszakitva";
  }
  const code = params.get("code");
  if (!code) {
    return "hiba";
  }

  const outcome = await linkDiscordAccount(felhasznaloId, code);
  switch (outcome.status) {
    case "in-use":
      return "foglalt";
    case "code-rejected":
      return "hiba";
    case "unavailable":
      return "nem-elerheto";
    case "linked":
      if (outcome.removedFelhasznaloId) {
        await endSessionsOf(outcome.removedFelhasznaloId);
      }
      return outcome.alreadyLinked ? "mar-a-tied" : "ok";
  }
}

/**
 * Go removed the empty Felhasználó the account was taken from: end the
 * sessions of its Better Auth user, which no longer means anyone. The link
 * already succeeded, so a failure here is only logged.
 */
async function endSessionsOf(userId: string): Promise<void> {
  try {
    const ctx = await auth.$context;
    await ctx.internalAdapter.deleteUserSessions(userId);
  } catch (err) {
    console.error("ending the sessions of a removed Felhasználó failed", err);
  }
}
