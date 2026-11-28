"use client";

import { useState } from "react";
import { useFormStatus } from "react-dom";
import { secondaryButtonClass } from "@/lib/ui";
import { handOverSzervezoAction } from "../actions";

type Utod = { felhasznaloId: string; name: string };

/**
 * The Szervező's "Szervezés átadása": pick a Tag who answered the Esemény,
 * then confirm inline. The new Szervező needs not accept; they get a DM.
 */
export function HandOverSzervezo({
  esemenyId,
  utodok,
}: {
  esemenyId: string;
  utodok: Utod[];
}) {
  const [open, setOpen] = useState(false);
  const [selected, setSelected] = useState("");
  const [confirming, setConfirming] = useState(false);
  const name = utodok.find((u) => u.felhasznaloId === selected)?.name ?? "";

  if (!open) {
    return (
      <div>
        <button
          type="button"
          onClick={() => setOpen(true)}
          className={secondaryButtonClass}
        >
          Szervezés átadása
        </button>
      </div>
    );
  }

  if (utodok.length === 0) {
    return (
      <div className="flex flex-col gap-2 text-sm">
        <p className="text-zinc-600 dark:text-zinc-400">
          A szervezést csak olyan tagnak adhatod át, aki válaszolt az eseményre,
          de rajtad kívül még senki nem válaszolt. Ha nem szervezed tovább,
          mondd le az eseményt.
        </p>
        <div>
          <button
            type="button"
            onClick={() => setOpen(false)}
            className={secondaryButtonClass}
          >
            Bezárás
          </button>
        </div>
      </div>
    );
  }

  return (
    <form
      action={handOverSzervezoAction.bind(null, esemenyId)}
      className="flex flex-col gap-2 rounded-2xl border border-zinc-300 p-3 dark:border-zinc-700"
    >
      <label htmlFor="uj-szervezo" className="text-sm font-medium">
        Szervezés átadása
      </label>
      <p className="text-sm text-zinc-600 dark:text-zinc-400">
        Csak olyan tagnak adhatod át, aki válaszolt az eseményre. Nem kell
        elfogadnia: azonnal ő lesz a szervező, és DM-ben értesítjük.
      </p>
      <select
        id="uj-szervezo"
        name="felhasznaloId"
        required
        value={selected}
        onChange={(e) => {
          setSelected(e.target.value);
          setConfirming(false);
        }}
        className="rounded-full border border-zinc-300 bg-transparent px-4 py-2 text-sm dark:border-zinc-700"
      >
        <option value="" disabled>
          Válassz tagot…
        </option>
        {utodok.map((u) => (
          <option key={u.felhasznaloId} value={u.felhasznaloId}>
            {u.name}
          </option>
        ))}
      </select>
      {confirming && selected !== "" ? (
        <div className="flex flex-col gap-2">
          <p className="text-sm">
            Biztosan átadod {name} nevű tagnak a szervezést? Utána te már nem
            véglegesíthetsz, nyithatod újra vagy mondhatod le az eseményt.
          </p>
          <div className="flex flex-wrap gap-2">
            <ConfirmButton />
            <button
              type="button"
              onClick={() => setConfirming(false)}
              className={secondaryButtonClass}
            >
              Mégse
            </button>
          </div>
        </div>
      ) : (
        <div className="flex flex-wrap gap-2">
          <button
            type="button"
            disabled={selected === ""}
            onClick={() => setConfirming(true)}
            className={`${secondaryButtonClass} disabled:opacity-50`}
          >
            Átadás…
          </button>
          <button
            type="button"
            onClick={() => {
              setOpen(false);
              setSelected("");
            }}
            className={secondaryButtonClass}
          >
            Mégse
          </button>
        </div>
      )}
    </form>
  );
}

function ConfirmButton() {
  const { pending } = useFormStatus();
  return (
    <button
      type="submit"
      disabled={pending}
      className="rounded-full bg-red-600 px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-80 disabled:opacity-60"
    >
      {pending ? "Átadás…" : "Igen, átadom"}
    </button>
  );
}
