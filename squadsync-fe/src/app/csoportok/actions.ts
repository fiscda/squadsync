"use server";

import { revalidatePath } from "next/cache";
import { redirect, unstable_rethrow } from "next/navigation";
import {
  BackendError,
  createCsoport,
  createCsoportMeghivo,
  leaveCsoport,
  removeTag,
  renameCsoport,
  revokeCsoportMeghivo,
  setCsoportMuted,
  setTagAdmin,
  unlinkCsatorna,
} from "@/lib/backend";
import { requireFelhasznalo } from "@/lib/felhasznalo";

export type CreateCsoportState = { error?: string };

/** Creates a Csoport; its creator becomes a Tag with the Admin role. */
export async function createCsoportAction(
  _previous: CreateCsoportState,
  formData: FormData,
): Promise<CreateCsoportState> {
  const felhasznalo = await requireFelhasznalo();
  const name = String(formData.get("name") ?? "").trim();
  if (name === "") {
    return { error: "Adj nevet a csoportnak." };
  }

  let csoportId: string;
  try {
    csoportId = (await createCsoport(felhasznalo.id, name)).id;
  } catch (err) {
    unstable_rethrow(err);
    if (err instanceof BackendError && err.status === 400) {
      return { error: "A csoport neve legfeljebb 60 karakter lehet." };
    }
    throw err;
  }
  redirect(`/csoportok/${csoportId}`);
}

/**
 * Makes a new Csoport-meghívó for a Csoport the caller is a Tag of. A 404
 * means the page is stale (the caller is no longer a Tag), so go back to the
 * list.
 */
export async function createMeghivoAction(csoportId: string) {
  const felhasznalo = await requireFelhasznalo();
  try {
    await createCsoportMeghivo(felhasznalo.id, csoportId);
  } catch (err) {
    unstable_rethrow(err);
    if (err instanceof BackendError && err.status === 404) {
      redirect("/csoportok");
    }
    throw err;
  }
  revalidatePath(`/csoportok/${csoportId}`);
}

/** Revokes a Csoport-meghívó: its link stops working at once. */
export async function revokeMeghivoAction(
  csoportId: string,
  meghivoId: string,
) {
  const felhasznalo = await requireFelhasznalo();
  try {
    await revokeCsoportMeghivo(felhasznalo.id, csoportId, meghivoId);
  } catch (err) {
    unstable_rethrow(err);
    if (err instanceof BackendError && err.status === 404) {
      redirect("/csoportok");
    }
    throw err;
  }
  revalidatePath(`/csoportok/${csoportId}`);
}

/**
 * Unlinks the Csoport's Discord channel. Only Admins see the button; a 403
 * or 404 means the page is stale, so just show it again.
 */
export async function unlinkCsatornaAction(csoportId: string) {
  const felhasznalo = await requireFelhasznalo();
  try {
    await unlinkCsatorna(felhasznalo.id, csoportId);
  } catch (err) {
    unstable_rethrow(err);
    if (err instanceof BackendError && err.status === 404) {
      redirect("/csoportok");
    }
    if (!(err instanceof BackendError && err.status === 403)) {
      throw err;
    }
  }
  revalidatePath(`/csoportok/${csoportId}`);
}

export type CsoportActionState = { error?: string };

const NOT_ADMIN = "Ehhez admin jogosultság kell.";
const LAST_ADMIN =
  "Te vagy az utolsó admin. Előbb adj admin rangot egy másik tagnak.";

/** Renames the Csoport. Admins only. */
export async function renameCsoportAction(
  csoportId: string,
  _previous: CsoportActionState,
  formData: FormData,
): Promise<CsoportActionState> {
  const felhasznalo = await requireFelhasznalo();
  const name = String(formData.get("name") ?? "").trim();
  if (name === "") {
    return { error: "Adj nevet a csoportnak." };
  }
  try {
    await renameCsoport(felhasznalo.id, csoportId, name);
  } catch (err) {
    unstable_rethrow(err);
    if (err instanceof BackendError) {
      if (err.status === 400) {
        return { error: "A csoport neve legfeljebb 60 karakter lehet." };
      }
      if (err.status === 403) {
        return { error: NOT_ADMIN };
      }
      if (err.status === 404) {
        redirect("/csoportok");
      }
    }
    throw err;
  }
  revalidatePath(`/csoportok/${csoportId}`);
  return {};
}

/**
 * Grants or revokes another Tag's Admin role. A 404 for the target means
 * they just left, so the refreshed page is the answer.
 */
export async function setAdminAction(
  csoportId: string,
  targetId: string,
  admin: boolean,
): Promise<CsoportActionState> {
  const felhasznalo = await requireFelhasznalo();
  try {
    await setTagAdmin(felhasznalo.id, csoportId, targetId, admin);
  } catch (err) {
    unstable_rethrow(err);
    if (err instanceof BackendError) {
      if (err.status === 409 && err.code === "last_admin") {
        return { error: LAST_ADMIN };
      }
      if (err.status === 403) {
        return { error: NOT_ADMIN };
      }
      if (err.status !== 404) {
        throw err;
      }
    } else {
      throw err;
    }
  }
  revalidatePath(`/csoportok/${csoportId}`);
  return {};
}

/** Removes a Tag from the Csoport. Admins only. */
export async function removeTagAction(
  csoportId: string,
  targetId: string,
): Promise<CsoportActionState> {
  const felhasznalo = await requireFelhasznalo();
  try {
    await removeTag(felhasznalo.id, csoportId, targetId);
  } catch (err) {
    unstable_rethrow(err);
    if (err instanceof BackendError) {
      if (err.status === 409 && err.code === "szervezo_of_active_esemeny") {
        return {
          error:
            "Ez a tag egy aktív esemény szervezője. Előbb adja át a szervezést, vagy mondja le az eseményt.",
        };
      }
      if (err.status === 409 && err.code === "last_admin") {
        return { error: LAST_ADMIN };
      }
      if (err.status === 403) {
        return { error: NOT_ADMIN };
      }
      if (err.status !== 404) {
        throw err;
      }
    } else {
      throw err;
    }
  }
  revalidatePath(`/csoportok/${csoportId}`);
  return {};
}

/** The caller leaves the Csoport, then goes back to their list. */
export async function leaveCsoportAction(
  csoportId: string,
): Promise<CsoportActionState> {
  const felhasznalo = await requireFelhasznalo();
  try {
    await leaveCsoport(felhasznalo.id, csoportId);
  } catch (err) {
    unstable_rethrow(err);
    if (err instanceof BackendError) {
      if (err.status === 409 && err.code === "szervezo_of_active_esemeny") {
        return {
          error: "Előbb add át a szervezést vagy mondd le az eseményt",
        };
      }
      if (err.status === 409 && err.code === "last_admin") {
        return { error: LAST_ADMIN };
      }
      if (err.status === 404) {
        // Already gone.
        redirect("/csoportok");
      }
    }
    throw err;
  }
  redirect("/csoportok");
}

/** Mutes or unmutes the caller's DMs from the Csoport. */
export async function setMutedAction(csoportId: string, muted: boolean) {
  const felhasznalo = await requireFelhasznalo();
  try {
    await setCsoportMuted(felhasznalo.id, csoportId, muted);
  } catch (err) {
    unstable_rethrow(err);
    if (err instanceof BackendError && err.status === 404) {
      redirect("/csoportok");
    }
    throw err;
  }
  revalidatePath(`/csoportok/${csoportId}`);
}
