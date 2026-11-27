import { redirect, unstable_rethrow } from "next/navigation";
import { joinWithToken } from "@/lib/csoport";

// The Discord login of a Csoport-meghívó comes back here (see
// signInForMeghivo). It makes the Felhasználó a Tag, then opens the Csoport.
// If the link was revoked meanwhile, the link's page explains.
//
// This GET has a side effect, but only on the caller's own session, and only
// for a link whose secret token they hold: a forged request can at most
// make the caller join a Csoport whose link the attacker knows. Joining as an
// existing Tag changes nothing.
export async function GET(
  _request: Request,
  ctx: RouteContext<"/meghivo/[token]/csatlakozas">,
) {
  const { token } = await ctx.params;
  const linkPage = `/meghivo/${encodeURIComponent(token)}`;

  let target: string;
  try {
    const result = await joinWithToken(token);
    target =
      result.status === "joined" || result.status === "already-tag"
        ? `/csoportok/${result.csoportId}`
        : linkPage;
  } catch (err) {
    unstable_rethrow(err); // let Next.js handle its own request-time signals
    // The link's page offers the join button again.
    console.error("joining a csoport after login failed", err);
    target = linkPage;
  }
  redirect(target);
}
