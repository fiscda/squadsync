"use client";

import { useState } from "react";
import { useFormStatus } from "react-dom";
import { secondaryButtonClass } from "@/lib/ui";
import { formatFt } from "./ft";

const inputClass =
  "rounded-lg border border-zinc-300 bg-transparent px-3 py-1.5 text-sm dark:border-zinc-700";

/**
 * Someone who can be in the split: id is their Felhasználó id, or
 * "v:<vendegId>" for a name-only Vendég (see resztvevoKey), who can't pay.
 */
export type KiadasFormPerson = { id: string; name: string; canPay: boolean };

export type KiadasFormDefaults = {
  description: string;
  /** null for an empty field. */
  amount: number | null;
  payerId: string;
  allapot: "tervezett" | "kifizetve";
  /** Who is checked in the split, with their custom amount. */
  reszek: Record<string, number>;
  egyedi: boolean;
};

/**
 * The form for adding or editing a Kiadás. With "egyedi összegek" on, it
 * shows an amount per checked person and keeps the submit button disabled
 * until they add up to the total (Go checks it again).
 */
export function KiadasForm({
  action,
  people,
  defaults,
  submitLabel,
}: {
  action: (formData: FormData) => Promise<void>;
  /** Who can be the payer or be in the split. */
  people: KiadasFormPerson[];
  defaults: KiadasFormDefaults;
  submitLabel: string;
}) {
  const [amount, setAmount] = useState(
    defaults.amount === null ? "" : String(defaults.amount),
  );
  const [checked, setChecked] = useState<Set<string>>(
    () => new Set(Object.keys(defaults.reszek)),
  );
  const [egyedi, setEgyedi] = useState(defaults.egyedi);
  const [custom, setCustom] = useState<Record<string, string>>(() =>
    Object.fromEntries(
      Object.entries(defaults.reszek).map(([id, n]) => [id, String(n)]),
    ),
  );

  const total = Number(amount);
  const sum = [...checked].reduce(
    (acc, id) => acc + (Number(custom[id] ?? "") || 0),
    0,
  );
  const sumOk = !egyedi || (Number.isInteger(total) && sum === total);
  const canSubmit = checked.size > 0 && sumOk;

  function toggle(id: string) {
    setChecked((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  }

  return (
    <form action={action} className="flex flex-col gap-3 text-sm">
      <div className="flex flex-wrap items-end gap-2">
        <label className="flex flex-col gap-1">
          Leírás
          <input
            name="description"
            required
            maxLength={200}
            defaultValue={defaults.description}
            placeholder="pl. szállás"
            className={inputClass}
          />
        </label>
        <label className="flex flex-col gap-1">
          Összeg (Ft)
          <input
            name="amount"
            type="number"
            required
            min={1}
            max={100000000}
            step={1}
            value={amount}
            onChange={(e) => setAmount(e.target.value)}
            className={`${inputClass} w-32`}
          />
        </label>
        <label className="flex flex-col gap-1">
          Ki fizet
          <select
            name="payerId"
            defaultValue={defaults.payerId}
            className={inputClass}
          >
            {people
              .filter((p) => p.canPay)
              .map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
          </select>
        </label>
      </div>

      <fieldset className="flex flex-wrap gap-4">
        <legend className="sr-only">Állapot</legend>
        <label className="flex items-center gap-2">
          <input
            type="radio"
            name="allapot"
            value="kifizetve"
            defaultChecked={defaults.allapot === "kifizetve"}
          />
          Kifizetve
        </label>
        <label className="flex items-center gap-2">
          <input
            type="radio"
            name="allapot"
            value="tervezett"
            defaultChecked={defaults.allapot === "tervezett"}
          />
          Tervezett (becsült összeg)
        </label>
      </fieldset>

      <fieldset className="flex flex-col gap-2">
        <legend className="mb-1 font-medium">Kik között oszlik meg</legend>
        <label className="flex items-center gap-2">
          <input
            type="checkbox"
            name="egyedi"
            checked={egyedi}
            onChange={(e) => setEgyedi(e.target.checked)}
          />
          Egyedi összegek
        </label>
        <ul className="flex flex-col gap-1">
          {people.map((p) => (
            <li key={p.id} className="flex flex-wrap items-center gap-2">
              <label className="flex min-w-32 items-center gap-2">
                <input
                  type="checkbox"
                  name="resz"
                  value={p.id}
                  checked={checked.has(p.id)}
                  onChange={() => toggle(p.id)}
                />
                {p.name}
              </label>
              {egyedi && checked.has(p.id) ? (
                <input
                  name={`osszeg_${p.id}`}
                  type="number"
                  min={0}
                  step={1}
                  required
                  value={custom[p.id] ?? ""}
                  onChange={(e) =>
                    setCustom((prev) => ({ ...prev, [p.id]: e.target.value }))
                  }
                  aria-label={`${p.name} része (Ft)`}
                  className={`${inputClass} w-28`}
                />
              ) : null}
            </li>
          ))}
        </ul>
        {egyedi ? (
          <p
            className={
              sumOk
                ? "text-zinc-600 dark:text-zinc-400"
                : "text-red-700 dark:text-red-400"
            }
          >
            Összesen {formatFt(sum)}
            {Number.isInteger(total) && total > 0
              ? ` / ${formatFt(total)}`
              : ""}
            {sumOk
              ? " ✓"
              : ": az egyedi összegeknek ki kell adniuk a teljes összeget."}
          </p>
        ) : null}
        {checked.size === 0 ? (
          <p className="text-red-700 dark:text-red-400">
            Legalább egy embert jelölj be.
          </p>
        ) : null}
      </fieldset>

      <div>
        <SubmitButton disabled={!canSubmit} label={submitLabel} />
      </div>
    </form>
  );
}

function SubmitButton({
  disabled,
  label,
}: {
  disabled: boolean;
  label: string;
}) {
  const { pending } = useFormStatus();
  return (
    <button
      type="submit"
      disabled={disabled || pending}
      className={`${secondaryButtonClass} disabled:opacity-50`}
    >
      {label}
    </button>
  );
}
