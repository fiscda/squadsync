"use server";

import { revalidatePath } from "next/cache";
import { redirect, unstable_rethrow } from "next/navigation";
import { BackendError, type Valasz } from "@/lib/backend";
import { requireFelhasznalo } from "@/lib/felhasznalo";
import {
  addVendeg,
  createEsemenyMeghivo,
  removeVendeg,
  revokeEsemenyMeghivo,
  setVendegVisszajelzes,
} from "@/lib/vendeg";

/**
 * Runs one Vendég change for the caller, then refreshes the Esemény page.
 * A refusal reloads it with ?hiba=vendeg_<reason> (see vendegHibaText); a
 * 404 means the page is stale (the Vendég or the access is gone).
 */
async function runVendegChange(
  esemenyId: string,
  change: (felhasznaloId: string) => Promise<unknown>,
) {
  const felhasznalo = await requireFelhasznalo();
  const page = `/esemenyek/${esemenyId}`;
  try {
    await change(felhasznalo.id);
  } catch (err) {
    unstable_rethrow(err);
    if (err instanceof BackendError) {
      const hiba =
        err.status === 404
          ? "vendeg_nincs"
          : err.status === 400
            ? "vendeg_ervenytelen"
            : err.code === "vendeg_closed" || err.code === "vendeg_has_kiadas"
              ? err.code
              : err.status === 403
                ? "vendeg_nincs_jog"
                : err.status === 409
                  ? "vendeg_allapot"
                  : null;
      if (hiba !== null) {
        revalidatePath(page);
        redirect(`${page}?hiba=${hiba}`);
      }
    }
    throw err;
  }
  revalidatePath(page);
  redirect(page);
}

/** Any Tag creates a new Esemény-meghívó link. */
export async function createEsemenyMeghivoAction(esemenyId: string) {
  await runVendegChange(esemenyId, (f) => createEsemenyMeghivo(f, esemenyId));
}

/** Any Tag revokes an Esemény-meghívó link. */
export async function revokeEsemenyMeghivoAction(
  esemenyId: string,
  meghivoId: string,
) {
  await runVendegChange(esemenyId, (f) =>
    revokeEsemenyMeghivo(f, esemenyId, meghivoId),
  );
}

/** Any Tag adds a name-only Vendég by the form's name. */
export async function addVendegAction(esemenyId: string, formData: FormData) {
  const name = String(formData.get("name") ?? "").trim();
  await runVendegChange(esemenyId, (f) => addVendeg(f, esemenyId, name));
}

/** The Vendég's Tag, or the Szervező, removes a Vendég. */
export async function removeVendegAction(esemenyId: string, vendegId: string) {
  await runVendegChange(esemenyId, (f) => removeVendeg(f, esemenyId, vendegId));
}

const valaszok: readonly string[] = ["igen", "talan", "nem"];

/** A Tag answers one Időpont for the name-only Vendég they added. */
export async function setVendegVisszajelzesAction(
  esemenyId: string,
  idopontId: string,
  vendegId: string,
  valasz: Valasz,
) {
  if (!valaszok.includes(valasz)) {
    throw new Error(`unknown valasz ${String(valasz)}`);
  }
  await runVendegChange(esemenyId, (f) =>
    setVendegVisszajelzes(f, esemenyId, idopontId, vendegId, valasz),
  );
}
