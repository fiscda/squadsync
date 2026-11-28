import { NextResponse } from "next/server";
import {
  encodeLinkState,
  isHttps,
  LINK_STATE_MAX_AGE,
  newLinkState,
} from "@/lib/discord-link";
import { getCurrentFelhasznalo } from "@/lib/felhasznalo";
import {
  GOOGLE_STATE_COOKIE,
  GOOGLE_STATE_PATH,
  googleAuthorizeUrl,
} from "@/lib/google-naptar";

// Starts connecting Google Calendar (see lib/google-naptar.ts). The "Google
// Naptár összekapcsolása" form on /fiok posts here.
export async function POST(request: Request) {
  const current = await getCurrentFelhasznalo();
  if (current.status !== "ok") {
    return NextResponse.redirect(new URL("/", request.url), 303);
  }

  const state = newLinkState(current.felhasznalo.id);
  const authorizeUrl = googleAuthorizeUrl(state.state);
  if (!authorizeUrl) {
    return NextResponse.redirect(
      new URL("/fiok?google=nem-elerheto", request.url),
      303,
    );
  }
  const response = NextResponse.redirect(authorizeUrl, 303);
  response.cookies.set(GOOGLE_STATE_COOKIE, encodeLinkState(state), {
    httpOnly: true,
    secure: isHttps(),
    // Lax: the cookie must come along on Google's top-level redirect back.
    sameSite: "lax",
    path: GOOGLE_STATE_PATH,
    maxAge: LINK_STATE_MAX_AGE,
  });
  return response;
}
