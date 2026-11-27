import "server-only";
import { revalidatePath } from "next/cache";
import { redirect, unstable_rethrow } from "next/navigation";
import { BackendError } from "@/lib/backend";
import { requireFelhasznalo } from "@/lib/felhasznalo";

/**
 * Runs one Kiegészítő change for the caller from a server action, then
 * refreshes the Esemény page. A refusal (400, 403, 409, or a 404 for
 * something that's gone) reloads the page with ?hiba=<Go's code>, which
 * kiegeszitoHibaText words. Every Kiegészítő's actions go through here.
 */
export async function runKiegeszitoChange(
  esemenyId: string,
  change: (felhasznaloId: string) => Promise<unknown>,
): Promise<void> {
  const felhasznalo = await requireFelhasznalo();
  const page = `/esemenyek/${esemenyId}`;
  try {
    await change(felhasznalo.id);
  } catch (err) {
    unstable_rethrow(err);
    if (err instanceof BackendError) {
      const hiba =
        err.status === 404
          ? "nincs"
          : err.status === 400
            ? "ervenytelen"
            : (err.code ?? "");
      if ([400, 403, 404, 409].includes(err.status) && hiba !== "") {
        revalidatePath(page);
        // A non-Tag gets the page's own 404.
        redirect(`${page}?hiba=${encodeURIComponent(hiba)}`);
      }
    }
    throw err;
  }
  revalidatePath(page);
}
