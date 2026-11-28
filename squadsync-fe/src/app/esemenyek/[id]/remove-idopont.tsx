"use client";

import { useState } from "react";
import { useFormStatus } from "react-dom";
import { secondaryButtonClass } from "@/lib/ui";
import { removeIdopontAction } from "../actions";

/**
 * The "Törlés" button of an Időpont. Removing one that has answers takes
 * its Visszajelzések with it (only the Szervező may), so that asks once
 * more inline.
 */
export function RemoveIdopont({
  esemenyId,
  idopontId,
  hasAnswers,
}: {
  esemenyId: string;
  idopontId: string;
  hasAnswers: boolean;
}) {
  const [confirming, setConfirming] = useState(false);
  const action = removeIdopontAction.bind(null, esemenyId, idopontId);

  if (hasAnswers && !confirming) {
    return (
      <button
        type="button"
        onClick={() => setConfirming(true)}
        className={`${secondaryButtonClass} text-red-700 dark:text-red-400`}
      >
        Törlés
      </button>
    );
  }
  return (
    <form action={action} className="flex flex-wrap items-center gap-2">
      {hasAnswers ? (
        <p className="text-sm">
          Az időponttal együtt a rá adott válaszok is törlődnek.
        </p>
      ) : null}
      <SubmitButton hasAnswers={hasAnswers} />
      {hasAnswers ? (
        <button
          type="button"
          onClick={() => setConfirming(false)}
          className={secondaryButtonClass}
        >
          Mégse
        </button>
      ) : null}
    </form>
  );
}

function SubmitButton({ hasAnswers }: { hasAnswers: boolean }) {
  const { pending } = useFormStatus();
  return (
    <button
      type="submit"
      disabled={pending}
      className={`${secondaryButtonClass} text-red-700 disabled:opacity-60 dark:text-red-400`}
    >
      {pending ? "Törlés…" : hasAnswers ? "Igen, törlöm" : "Törlés"}
    </button>
  );
}
