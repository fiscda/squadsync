"use client";

import { useActionState, useEffect, useState } from "react";
import { buttonClass, secondaryButtonClass } from "@/lib/ui";
import { type EditEsemenyState, editEsemenyAction } from "../actions";
import { inputClass } from "../idopont-rows";

/**
 * The Szervező's "Szerkesztés": an inline form for the title, description
 * and Helyszín. The Időpontok' times are never edited here: add a new
 * Időpont and remove the old one, or reopen Egyeztetés.
 */
export function EditEsemeny({
  esemenyId,
  title,
  description,
  helyszinName,
  helyszinUrl,
}: {
  esemenyId: string;
  title: string;
  description: string;
  helyszinName: string;
  helyszinUrl: string;
}) {
  const [open, setOpen] = useState(false);
  // Controlled, so that a refused save keeps what was typed.
  const [fields, setFields] = useState({
    title,
    description,
    helyszinName,
    helyszinUrl,
  });
  const setField = (name: keyof typeof fields, value: string) =>
    setFields((f) => ({ ...f, [name]: value }));
  const [state, action, pending] = useActionState<EditEsemenyState, FormData>(
    editEsemenyAction.bind(null, esemenyId),
    {},
  );

  // A saved edit closes the form; the page shows the new details.
  useEffect(() => {
    if (state.saved) {
      setOpen(false);
    }
  }, [state]);

  if (!open) {
    return (
      <button
        type="button"
        onClick={() => {
          // Start from the current details every time.
          setFields({ title, description, helyszinName, helyszinUrl });
          setOpen(true);
        }}
        className={`${secondaryButtonClass} self-start`}
      >
        Szerkesztés
      </button>
    );
  }
  return (
    <form
      action={action}
      className="flex flex-col gap-4 rounded-2xl border border-zinc-200 p-4 dark:border-zinc-800"
    >
      <label className="flex flex-col gap-1">
        <span className="font-medium">Cím</span>
        <input
          name="title"
          required
          maxLength={100}
          autoComplete="off"
          value={fields.title}
          onChange={(e) => setField("title", e.target.value)}
          className={inputClass}
        />
      </label>
      <label className="flex flex-col gap-1">
        <span className="font-medium">
          Leírás <span className="text-sm text-zinc-500">(nem kötelező)</span>
        </span>
        <textarea
          name="description"
          maxLength={2000}
          rows={3}
          value={fields.description}
          onChange={(e) => setField("description", e.target.value)}
          className={inputClass}
        />
      </label>
      <fieldset className="flex flex-col gap-3">
        <legend className="mb-1 font-medium">
          Helyszín <span className="text-sm text-zinc-500">(nem kötelező)</span>
        </legend>
        <input
          name="helyszinName"
          maxLength={200}
          placeholder="Pl. Anna lakása"
          aria-label="Helyszín neve"
          autoComplete="off"
          value={fields.helyszinName}
          onChange={(e) => setField("helyszinName", e.target.value)}
          className={inputClass}
        />
        <input
          name="helyszinUrl"
          type="url"
          maxLength={2000}
          placeholder="Google Maps link (https://…)"
          aria-label="Google Maps link"
          autoComplete="off"
          value={fields.helyszinUrl}
          onChange={(e) => setField("helyszinUrl", e.target.value)}
          className={inputClass}
        />
      </fieldset>
      {state.error ? (
        <p role="alert" className="text-sm text-red-600 dark:text-red-400">
          {state.error}
        </p>
      ) : null}
      <div className="flex flex-wrap gap-2">
        <button type="submit" disabled={pending} className={buttonClass}>
          {pending ? "Mentés…" : "Mentés"}
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
