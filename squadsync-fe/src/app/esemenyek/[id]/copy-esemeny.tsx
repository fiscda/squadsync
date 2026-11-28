"use client";

import { useState } from "react";
import { useFormStatus } from "react-dom";
import { secondaryButtonClass } from "@/lib/ui";
import { copyEsemenyAction } from "../actions";

/**
 * "Másolás egy héttel későbbre". Every copy is announced to the whole
 * Csoport, so it asks once more inline, and the button is disabled while
 * the copy is being made.
 */
export function CopyEsemeny({ esemenyId }: { esemenyId: string }) {
  const [confirming, setConfirming] = useState(false);

  if (!confirming) {
    return (
      <button
        type="button"
        onClick={() => setConfirming(true)}
        className={`${secondaryButtonClass} self-start`}
      >
        Másolás egy héttel későbbre
      </button>
    );
  }
  return (
    <form
      action={copyEsemenyAction.bind(null, esemenyId)}
      className="flex flex-wrap items-center gap-2"
    >
      <p className="text-sm">
        Biztosan? Az új eseményről az egész csoport értesítést kap.
      </p>
      <SubmitButton />
      <button
        type="button"
        onClick={() => setConfirming(false)}
        className={secondaryButtonClass}
      >
        Mégse
      </button>
    </form>
  );
}

function SubmitButton() {
  const { pending } = useFormStatus();
  return (
    <button
      type="submit"
      disabled={pending}
      className={`${secondaryButtonClass} disabled:opacity-60`}
    >
      {pending ? "Másolás…" : "Igen, másolom"}
    </button>
  );
}
