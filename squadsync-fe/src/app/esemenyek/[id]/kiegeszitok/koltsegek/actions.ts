"use server";

import {
  addKiadas,
  closeElszamolas,
  confirmKiadasok,
  deleteKiadas,
  type KiadasInput,
  reopenElszamolas,
  setUtalasMegvolt,
  unconfirmKiadasok,
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
      // A name-only Vendég's key is "v:<vendegId>" (see resztvevoKey).
      reszek: formData.getAll("resz").map((value) => {
        const id = String(value);
        const vendeg = id.startsWith("v:");
        return {
          felhasznaloId: vendeg ? "" : id,
          vendegId: vendeg ? id.slice(2) : "",
          amount: egyedi ? wholeNumber(formData.get(`osszeg_${id}`)) : 0,
        };
      }),
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

export async function confirmKiadasokAction(esemenyId: string) {
  await runKiegeszitoChange(esemenyId, (felhasznaloId) =>
    confirmKiadasok(felhasznaloId, esemenyId),
  );
}

export async function unconfirmKiadasokAction(esemenyId: string) {
  await runKiegeszitoChange(esemenyId, (felhasznaloId) =>
    unconfirmKiadasok(felhasznaloId, esemenyId),
  );
}

export async function closeElszamolasAction(esemenyId: string) {
  await runKiegeszitoChange(esemenyId, (felhasznaloId) =>
    closeElszamolas(felhasznaloId, esemenyId),
  );
}

export async function reopenElszamolasAction(esemenyId: string) {
  await runKiegeszitoChange(esemenyId, (felhasznaloId) =>
    reopenElszamolas(felhasznaloId, esemenyId),
  );
}

export async function setUtalasMegvoltAction(
  esemenyId: string,
  utalasId: string,
  megvolt: boolean,
) {
  await runKiegeszitoChange(esemenyId, (felhasznaloId) =>
    setUtalasMegvolt(felhasznaloId, esemenyId, utalasId, megvolt),
  );
}
