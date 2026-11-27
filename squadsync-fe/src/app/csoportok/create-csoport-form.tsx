"use client";

import { useActionState } from "react";
import { buttonClass } from "@/lib/ui";
import { type CreateCsoportState, createCsoportAction } from "./actions";

export function CreateCsoportForm() {
  const [state, action, pending] = useActionState<CreateCsoportState, FormData>(
    createCsoportAction,
    {},
  );

  return (
    <form action={action} className="flex flex-col gap-3">
      <label htmlFor="name" className="font-medium">
        Új csoport létrehozása
      </label>
      <div className="flex flex-col gap-3 sm:flex-row">
        <input
          id="name"
          name="name"
          required
          maxLength={60}
          placeholder="Pl. Péntek esték"
          autoComplete="off"
          className="min-w-0 flex-1 rounded-full border border-zinc-300 bg-transparent px-4 py-3 dark:border-zinc-700"
        />
        <button type="submit" disabled={pending} className={buttonClass}>
          {pending ? "Létrehozás…" : "Létrehozás"}
        </button>
      </div>
      {state.error ? (
        <p role="alert" className="text-sm text-red-600 dark:text-red-400">
          {state.error}
        </p>
      ) : null}
    </form>
  );
}
