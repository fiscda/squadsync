import { redirect, unstable_rethrow } from "next/navigation";
import { joinEsemenyWithToken } from "@/lib/vendeg";

// The Discord login of an Esemény-meghívó comes back here (see
// signInForEsemenyMeghivo). It makes the Felhasználó a Vendég of that
// Esemény (a Tag, or an existing Vendég, just lands on it), then opens the
// Esemény. If the link stopped working meanwhile, the link's page explains.
//
// This GET has a side effect, but only on the caller's own session, and only
// for a link whose secret token they hold: a forged request can at most make
// the caller a Vendég of an Esemény whose link the attacker knows. Joining
// again changes nothing.
export async function GET(
  _request: Request,
  ctx: RouteContext<"/esemeny-meghivo/[token]/csatlakozas">,
) {
  const { token } = await ctx.params;
  const linkPage = `/esemeny-meghivo/${encodeURIComponent(token)}`;

  let target: string;
  try {
    const result = await joinEsemenyWithToken(token);
    target =
      result.status === "ok" ? `/esemenyek/${result.esemenyId}` : linkPage;
  } catch (err) {
    unstable_rethrow(err); // let Next.js handle its own request-time signals
    // The link's page offers the join button again.
    console.error("joining an esemeny after login failed", err);
    target = linkPage;
  }
  redirect(target);
}
