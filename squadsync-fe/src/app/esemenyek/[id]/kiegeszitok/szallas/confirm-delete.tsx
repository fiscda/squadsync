"use client";

import { useState } from "react";
import { useFormStatus } from "react-dom";
import { secondaryButtonClass } from "@/lib/ui";

const dangerClass = `${secondaryButtonClass} text-red-700 dark:text-red-400`;

/**
 * A delete button that asks once more inline before it submits. The action
 * is a server action already bound to what it deletes.
 */
export function ConfirmDelete({
  action,
  label,
  warning,
}: {
  action: () => Promise<void>;
  label: string;
  warning: string;
}) {
  const [confirming, setConfirming] = useState(false);

  if (!confirming) {
    return (
      <button
        type="button"
        onClick={() => setConfirming(true)}
        className={dangerClass}
      >
        {label}
      </button>
    );
  }
  return (
    <form action={action} className="flex flex-wrap items-center gap-2">
      <p className="text-sm">{warning}</p>
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
      className={`${dangerClass} disabled:opacity-60`}
    >
      {pending ? "Törlés…" : "Igen, törlöm"}
    </button>
  );
}
