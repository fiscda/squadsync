"use server";

import {
  addKiadas,
  deleteKiadas,
  type KiadasInput,
  updateKiadas,
} from "@/lib/kiegeszito/koltsegek";
import { runKiegeszitoChange } from "../run-change";

function wholeNumber(value: unknown): number {
  // Go refuses anything out of range with 400; NaN would be sent as null.
  const n = Number(String(value ?? "").trim());
  return Number.isInteger(n) ? n : -1;
}

/**
 * Reads a Kiadás form (see kiadas-form.tsx): every checked "resz" is in the
 * split; with "egyedi" on, each one's "osszeg_<id>" is their amount. Go
 * checks the rest, including that custom amounts add up.
 */
function kiadasInput(formData: FormData): KiadasInput {
  const egyedi = formData.get("egyedi") === "on";
  return {
    description: String(formData.get("description") ?? "").trim(),
    amount: wholeNumber(formData.get("amount")),
    payerId: String(formData.get("payerId") ?? ""),
    allapot:
      formData.get("allapot") === "tervezett" ? "tervezett" : "kifizetve",
    split: {
      egyenlo: !egyedi,
      reszek: formData.getAll("resz").map((id) => ({
        felhasznaloId: String(id),
        vendegId: "",
        amount: egyedi ? wholeNumber(formData.get(`osszeg_${id}`)) : 0,
      })),
    },
  };
}

export async function addKiadasAction(esemenyId: string, formData: FormData) {
  const input = kiadasInput(formData);
  await runKiegeszitoChange(esemenyId, (felhasznaloId) =>
    addKiadas(felhasznaloId, esemenyId, input),
  );
}

export async function updateKiadasAction(
  esemenyId: string,
  kiadasId: string,
  formData: FormData,
) {
  const input = kiadasInput(formData);
  await runKiegeszitoChange(esemenyId, (felhasznaloId) =>
    updateKiadas(felhasznaloId, esemenyId, kiadasId, input),
  );
}

/**
 * Marks a planned Kiadás paid with its real amount, keeping everything else:
 * the description and payer ("" keeps them), and the split (an equal one is
 * recomputed; a custom one sends its current amount, see panel.tsx).
 */
export async function markKifizetveAction(
  esemenyId: string,
  kiadasId: string,
  formData: FormData,
) {
  const input: KiadasInput = {
    description: "",
    amount: wholeNumber(formData.get("amount")),
    payerId: "",
    allapot: "kifizetve",
    split: null,
  };
  await runKiegeszitoChange(esemenyId, (felhasznaloId) =>
    updateKiadas(felhasznaloId, esemenyId, kiadasId, input),
  );
}

export async function deleteKiadasAction(esemenyId: string, kiadasId: string) {
  await runKiegeszitoChange(esemenyId, (felhasznaloId) =>
    deleteKiadas(felhasznaloId, esemenyId, kiadasId),
  );
}
