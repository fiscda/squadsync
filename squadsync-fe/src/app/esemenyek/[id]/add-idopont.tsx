"use client";

import { useActionState, useEffect, useState } from "react";
import { buttonClass, secondaryButtonClass } from "@/lib/ui";
import { type AddIdopontState, addIdopontAction } from "../actions";
import { IdopontRows } from "../idopont-rows";

/**
 * "+ Időpont hozzáadása": any Tag adds one more Időpont during Egyeztetés,
 * with the same fields as when proposing.
 */
export function AddIdopont({ esemenyId }: { esemenyId: string }) {
  const [open, setOpen] = useState(false);
  // Changes after each added Időpont, to remount the fields empty.
  const [round, setRound] = useState(0);
  const [state, action, pending] = useActionState<AddIdopontState, FormData>(
    addIdopontAction.bind(null, esemenyId),
    {},
  );

  useEffect(() => {
    if (state.added) {
      setOpen(false);
      setRound((r) => r + 1);
    }
  }, [state]);

  if (!open) {
    return (
      <button
        type="button"
        onClick={() => setOpen(true)}
        className={`${secondaryButtonClass} self-start`}
      >
        + Időpont hozzáadása
      </button>
    );
  }
  return (
    <form action={action} className="flex flex-col gap-4">
      <IdopontRows key={round} single />
      {state.error ? (
        <p role="alert" className="text-sm text-red-600 dark:text-red-400">
          {state.error}
        </p>
      ) : null}
      <div className="flex flex-wrap gap-2">
        <button type="submit" disabled={pending} className={buttonClass}>
          {pending ? "Hozzáadás…" : "Hozzáadás"}
        </button>
        <button
          type="button"
          onClick={() => setOpen(false)}
          className={secondaryButtonClass}
        >
          Mégse
        </button>
      </div>
    </form>
  );
}
