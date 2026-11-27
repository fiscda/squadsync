"use client";

import { useState } from "react";
import { useFormStatus } from "react-dom";
import { secondaryButtonClass } from "@/lib/ui";
import { cancelEsemenyAction } from "../actions";

/**
 * The Szervező's "Lemondás" button. Cancelling is final, so it asks once
 * more inline before submitting.
 */
export function CancelEsemeny({ esemenyId }: { esemenyId: string }) {
  const [confirming, setConfirming] = useState(false);

  if (!confirming) {
    return (
      <button
        type="button"
        onClick={() => setConfirming(true)}
        className={`${secondaryButtonClass} text-red-700 dark:text-red-400`}
      >
        Lemondás
      </button>
    );
  }
  return (
    <form
      action={cancelEsemenyAction.bind(null, esemenyId)}
      className="flex flex-col gap-2 rounded-2xl border border-red-300 p-3 dark:border-red-900"
    >
      <p className="text-sm">
        Biztosan lemondod? Az esemény elmarad, ezt nem lehet visszavonni, és
        mindenki értesítést kap róla.
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
      {pending ? "Lemondás…" : "Igen, lemondom"}
    </button>
  );
}
