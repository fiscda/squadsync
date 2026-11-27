"use server";

import { revalidatePath } from "next/cache";
import { redirect, unstable_rethrow } from "next/navigation";
import {
  BackendError,
  createCsoport,
  createCsoportMeghivo,
  revokeCsoportMeghivo,
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
