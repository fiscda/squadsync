import { cookies } from "next/headers";
import { redirect, unstable_rethrow } from "next/navigation";
import { connectGoogleNaptar } from "@/lib/backend";
import { linkStateMatches } from "@/lib/discord-link";
import { getCurrentFelhasznalo } from "@/lib/felhasznalo";
import { GOOGLE_STATE_COOKIE, GOOGLE_STATE_PATH } from "@/lib/google-naptar";

/** The outcomes /fiok explains, passed as `?google=`. */
type GoogleResult = "ok" | "megszakitva" | "nem-elerheto" | "hiba";

// Google sends the browser back here at the end of connecting Google
// Calendar (see lib/google-naptar.ts). The state must match the httpOnly
// cookie set for this Felhasználó when the flow started, which stops a
// forged callback from connecting someone else's calendar to them.
export async function GET(request: Request) {
  const params = new URL(request.url).searchParams;
  const cookieStore = await cookies();
  const stored = cookieStore.get(GOOGLE_STATE_COOKIE)?.value;
  // One use only, whatever happens next.
  cookieStore.delete({ name: GOOGLE_STATE_COOKIE, path: GOOGLE_STATE_PATH });

  const current = await getCurrentFelhasznalo();
  if (current.status !== "ok") {
    redirect("/");
  }
  const { felhasznalo } = current;

  let result: GoogleResult;
  try {
    result = await finishConnecting(felhasznalo.id, stored, params);
  } catch (err) {
    unstable_rethrow(err); // let Next.js handle its own request-time signals
    console.error("connecting Google Calendar failed", err);
    result = "hiba";
  }
  redirect(`/fiok?google=${result}`);
}

async function finishConnecting(
  felhasznaloId: string,
  stored: string | undefined,
  params: URLSearchParams,
): Promise<GoogleResult> {
  if (!linkStateMatches(stored, params.get("state"), felhasznaloId)) {
    return "hiba";
  }
  // The user pressed Cancel on Google's page.
  if (params.get("error")) {
    return "megszakitva";
  }
  const code = params.get("code");
  if (!code) {
    return "hiba";
  }
  switch (await connectGoogleNaptar(felhasznaloId, code)) {
    case "connected":
      return "ok";
    case "unavailable":
      return "nem-elerheto";
    case "code-rejected":
      return "hiba";
  }
}
