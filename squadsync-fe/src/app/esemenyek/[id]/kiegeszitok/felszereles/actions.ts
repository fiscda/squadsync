"use server";

import {
  addTetel,
  deleteTetel,
  setVallalas,
  type TetelInput,
  updateTetel,
} from "@/lib/kiegeszito/felszereles";
import { runKiegeszitoChange } from "../run-change";

/** Reads a Tétel form. An empty quantity means 1; Go checks the rest. */
function tetelInput(formData: FormData): TetelInput {
  const quantity = String(formData.get("quantity") ?? "").trim();
  return {
    name: String(formData.get("name") ?? "").trim(),
    quantity: quantity === "" ? 1 : Number(quantity),
    mindenkiHozza: formData.get("mindenkiHozza") === "on",
  };
}

function wholeNumber(value: number): number {
  // Go refuses anything out of range with 400; NaN would be sent as null.
  return Number.isInteger(value) ? value : -1;
}

export async function addTetelAction(esemenyId: string, formData: FormData) {
  const input = tetelInput(formData);
  input.quantity = wholeNumber(input.quantity);
  await runKiegeszitoChange(esemenyId, (felhasznaloId) =>
    addTetel(felhasznaloId, esemenyId, input),
  );
}

export async function updateTetelAction(
  esemenyId: string,
  tetelId: string,
  formData: FormData,
) {
  const input = tetelInput(formData);
  input.quantity = wholeNumber(input.quantity);
  await runKiegeszitoChange(esemenyId, (felhasznaloId) =>
    updateTetel(felhasznaloId, esemenyId, tetelId, input),
  );
}

export async function deleteTetelAction(esemenyId: string, tetelId: string) {
  await runKiegeszitoChange(esemenyId, (felhasznaloId) =>
    deleteTetel(felhasznaloId, esemenyId, tetelId),
  );
}

/**
 * Sets the caller's claim on a Tétel, or, with the form's vendegId, their
 * name-only Vendég's; the form's quantity 0 removes it.
 */
export async function setVallalasAction(
  esemenyId: string,
  tetelId: string,
  formData: FormData,
) {
  const quantity = wholeNumber(Number(formData.get("quantity") ?? ""));
  const vendegId = String(formData.get("vendegId") ?? "");
  await runKiegeszitoChange(esemenyId, (felhasznaloId) =>
    setVallalas(felhasznaloId, esemenyId, tetelId, quantity, vendegId),
  );
}
