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

/** Makes a new Csoport-meghívó for a Csoport the caller is a Tag of. */
export async function createMeghivoAction(csoportId: string) {
  const felhasznalo = await requireFelhasznalo();
  await createCsoportMeghivo(felhasznalo.id, csoportId);
  revalidatePath(`/csoportok/${csoportId}`);
}

/** Revokes a Csoport-meghívó: its link stops working at once. */
export async function revokeMeghivoAction(
  csoportId: string,
  meghivoId: string,
) {
  const felhasznalo = await requireFelhasznalo();
  await revokeCsoportMeghivo(felhasznalo.id, csoportId, meghivoId);
  revalidatePath(`/csoportok/${csoportId}`);
}
