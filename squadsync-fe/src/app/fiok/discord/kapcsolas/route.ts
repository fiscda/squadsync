import { NextResponse } from "next/server";
import {
  discordAuthorizeUrl,
  encodeLinkState,
  isHttps,
  LINK_STATE_COOKIE,
  LINK_STATE_MAX_AGE,
  LINK_STATE_PATH,
  newLinkState,
} from "@/lib/discord-link";
import { getCurrentFelhasznalo } from "@/lib/felhasznalo";

// Starts linking a further Discord account (see lib/discord-link.ts). The
// "Másik Discord fiók kapcsolása" form on /fiok posts here.
export async function POST(request: Request) {
  const current = await getCurrentFelhasznalo();
  if (current.status !== "ok") {
    return NextResponse.redirect(new URL("/", request.url), 303);
  }

  const linkState = newLinkState(current.felhasznalo.id);
  const response = NextResponse.redirect(
    discordAuthorizeUrl(linkState.state),
    303,
  );
  response.cookies.set(LINK_STATE_COOKIE, encodeLinkState(linkState), {
    httpOnly: true,
    secure: isHttps(),
    // Lax: the cookie must come along on Discord's top-level redirect back.
    sameSite: "lax",
    path: LINK_STATE_PATH,
    maxAge: LINK_STATE_MAX_AGE,
  });
  return response;
}
