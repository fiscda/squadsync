import { redirect, unstable_rethrow } from "next/navigation";
import { ensureFelhasznalo } from "@/lib/felhasznalo";

// Better Auth sends the browser here after every Discord login (the
// callbackURL in actions.ts). It makes sure Go knows the Felhasználó, then
// goes to the home page.
//
// This GET has a side effect, but only an idempotent one on the caller's own
// session: a forged request can at most create the caller's own Felhasználó,
// which their login already did.
export async function GET() {
  try {
    await ensureFelhasznalo();
  } catch (err) {
    unstable_rethrow(err); // let Next.js handle its own request-time signals
    // The home page offers a retry when the Felhasználó is still missing.
    console.error("ensureFelhasznalo after login failed", err);
  }
  redirect("/");
}
