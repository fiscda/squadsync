"use server";

import {
  type AutoInput,
  claimUles,
  deleteAuto,
  moveUtas,
  offerAuto,
  releaseUles,
  setSajatUton,
  updateAuto,
} from "@/lib/kiegeszito/autok";
import { runKiegeszitoChange } from "../run-change";

/** Reads an Autó form. Go checks the range; NaN would be sent as null. */
function autoInput(formData: FormData): AutoInput {
  const seats = Number(String(formData.get("seats") ?? "").trim());
  return {
    seats: Number.isInteger(seats) ? seats : -1,
    note: String(formData.get("note") ?? "").trim(),
  };
}

export async function offerAutoAction(esemenyId: string, formData: FormData) {
  const input = autoInput(formData);
  await runKiegeszitoChange(esemenyId, (felhasznaloId) =>
    offerAuto(felhasznaloId, esemenyId, input),
  );
}

export async function updateAutoAction(
  esemenyId: string,
  autoId: string,
  formData: FormData,
) {
  const input = autoInput(formData);
  await runKiegeszitoChange(esemenyId, (felhasznaloId) =>
    updateAuto(felhasznaloId, esemenyId, autoId, input),
  );
}

export async function deleteAutoAction(esemenyId: string, autoId: string) {
  await runKiegeszitoChange(esemenyId, (felhasznaloId) =>
    deleteAuto(felhasznaloId, esemenyId, autoId),
  );
}

/** "Beszállok": the caller claims a seat. */
export async function claimUlesAction(esemenyId: string, autoId: string) {
  await runKiegeszitoChange(esemenyId, (felhasznaloId) =>
    claimUles(felhasznaloId, esemenyId, autoId),
  );
}

/**
 * "Kiszállok" (no target), or the Szervező removing someone from their seat.
 */
export async function releaseUlesAction(esemenyId: string, targetId: string) {
  await runKiegeszitoChange(esemenyId, (felhasznaloId) =>
    releaseUles(felhasznaloId, esemenyId, targetId || undefined),
  );
}

/** The Szervező moves someone to the Autó picked in the form. */
export async function moveUtasAction(
  esemenyId: string,
  targetId: string,
  formData: FormData,
) {
  const autoId = String(formData.get("autoId") ?? "");
  await runKiegeszitoChange(esemenyId, (felhasznaloId) =>
    moveUtas(felhasznaloId, esemenyId, autoId, targetId),
  );
}

export async function setSajatUtonAction(esemenyId: string, on: boolean) {
  await runKiegeszitoChange(esemenyId, (felhasznaloId) =>
    setSajatUton(felhasznaloId, esemenyId, on),
  );
}
