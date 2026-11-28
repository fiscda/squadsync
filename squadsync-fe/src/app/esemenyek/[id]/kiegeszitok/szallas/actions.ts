"use server";

import {
  addAgy,
  addSzoba,
  claimAgy,
  deleteAgy,
  deleteSzoba,
  moveAlvo,
  releaseFoglalas,
  renameSzoba,
  setMasholAlszik,
  setSzallasNote,
  updateAgy,
} from "@/lib/kiegeszito/szallas";
import { runKiegeszitoChange } from "../run-change";

/** Reads an Ágy form. Go checks the range; NaN would be sent as null. */
function agyInput(formData: FormData) {
  const capacity = Number(String(formData.get("capacity") ?? "").trim());
  return {
    capacity: Number.isInteger(capacity) ? capacity : -1,
    label: String(formData.get("label") ?? "").trim(),
  };
}

function text(formData: FormData, name: string): string {
  return String(formData.get(name) ?? "").trim();
}

export async function setSzallasNoteAction(
  esemenyId: string,
  formData: FormData,
) {
  const note = text(formData, "note");
  await runKiegeszitoChange(esemenyId, (felhasznaloId) =>
    setSzallasNote(felhasznaloId, esemenyId, note),
  );
}

export async function addSzobaAction(esemenyId: string, formData: FormData) {
  const name = text(formData, "name");
  await runKiegeszitoChange(esemenyId, (felhasznaloId) =>
    addSzoba(felhasznaloId, esemenyId, name),
  );
}

export async function renameSzobaAction(
  esemenyId: string,
  szobaId: string,
  formData: FormData,
) {
  const name = text(formData, "name");
  await runKiegeszitoChange(esemenyId, (felhasznaloId) =>
    renameSzoba(felhasznaloId, esemenyId, szobaId, name),
  );
}

export async function deleteSzobaAction(esemenyId: string, szobaId: string) {
  await runKiegeszitoChange(esemenyId, (felhasznaloId) =>
    deleteSzoba(felhasznaloId, esemenyId, szobaId),
  );
}

export async function addAgyAction(
  esemenyId: string,
  szobaId: string,
  formData: FormData,
) {
  const input = agyInput(formData);
  await runKiegeszitoChange(esemenyId, (felhasznaloId) =>
    addAgy(felhasznaloId, esemenyId, szobaId, input),
  );
}

export async function updateAgyAction(
  esemenyId: string,
  agyId: string,
  formData: FormData,
) {
  const input = agyInput(formData);
  await runKiegeszitoChange(esemenyId, (felhasznaloId) =>
    updateAgy(felhasznaloId, esemenyId, agyId, input),
  );
}

export async function deleteAgyAction(esemenyId: string, agyId: string) {
  await runKiegeszitoChange(esemenyId, (felhasznaloId) =>
    deleteAgy(felhasznaloId, esemenyId, agyId),
  );
}

/** "Ide alszom": the caller takes a spot. */
export async function claimAgyAction(esemenyId: string, agyId: string) {
  await runKiegeszitoChange(esemenyId, (felhasznaloId) =>
    claimAgy(felhasznaloId, esemenyId, agyId),
  );
}

/**
 * "Kiköltözöm" (no target), or the Szervező removing someone from their
 * spot.
 */
export async function releaseFoglalasAction(
  esemenyId: string,
  targetId: string,
) {
  await runKiegeszitoChange(esemenyId, (felhasznaloId) =>
    releaseFoglalas(felhasznaloId, esemenyId, targetId || undefined),
  );
}

/** The Szervező moves someone to the Ágy picked in the form. */
export async function moveAlvoAction(
  esemenyId: string,
  targetId: string,
  formData: FormData,
) {
  const agyId = text(formData, "agyId");
  await runKiegeszitoChange(esemenyId, (felhasznaloId) =>
    moveAlvo(felhasznaloId, esemenyId, agyId, targetId),
  );
}

export async function setMasholAlszikAction(esemenyId: string, on: boolean) {
  await runKiegeszitoChange(esemenyId, (felhasznaloId) =>
    setMasholAlszik(felhasznaloId, esemenyId, on),
  );
}
