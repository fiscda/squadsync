"use client";

import { useActionState, useState } from "react";
import {
  type ProposeEsemenyState,
  proposeEsemenyAction,
} from "@/app/esemenyek/actions";
import { IdopontRows, inputClass } from "@/app/esemenyek/idopont-rows";
import { buttonClass } from "@/lib/ui";

/**
 * The propose form. A client component so that inputs are controlled and a
 * refused submit keeps them.
 */
export function EsemenyForm({ csoportId }: { csoportId: string }) {
  const [state, action, pending] = useActionState<
    ProposeEsemenyState,
    FormData
  >(proposeEsemenyAction.bind(null, csoportId), {});
  const [fields, setFields] = useState({
    title: "",
    description: "",
    helyszinName: "",
    helyszinUrl: "",
    hataridoDate: "",
    hataridoTime: "",
  });

  const setField = (name: keyof typeof fields, value: string) =>
    setFields((f) => ({ ...f, [name]: value }));

  return (
    <form action={action} className="flex flex-col gap-6">
      <label className="flex flex-col gap-1">
        <span className="font-medium">Cím</span>
        <input
          name="title"
          required
          maxLength={100}
          placeholder="Pl. Társasjáték-est"
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

      <IdopontRows />

      <fieldset className="flex flex-col gap-2">
        <legend className="mb-1 font-medium">
          Szavazási határidő{" "}
          <span className="text-sm text-zinc-500">(nem kötelező)</span>
        </legend>
        <p className="text-sm text-zinc-600 dark:text-zinc-400">
          Budapesti idő szerint. Amikor lejár, Discordon üzenetet kapsz az
          időpontok állásáról. A szavazás ezután is nyitva marad.
        </p>
        <div className="flex flex-wrap gap-2">
          <input
            type="date"
            name="hataridoDate"
            aria-label="Határidő napja"
            value={fields.hataridoDate}
            onChange={(e) => setField("hataridoDate", e.target.value)}
            className={inputClass}
          />
          <input
            type="time"
            name="hataridoTime"
            aria-label="Határidő ideje"
            value={fields.hataridoTime}
            onChange={(e) => setField("hataridoTime", e.target.value)}
            className={inputClass}
          />
        </div>
      </fieldset>

      {state.error ? (
        <p role="alert" className="text-sm text-red-600 dark:text-red-400">
          {state.error}
        </p>
      ) : null}
      <button
        type="submit"
        disabled={pending}
        className={`${buttonClass} self-start`}
      >
        {pending ? "Létrehozás…" : "Esemény létrehozása"}
      </button>
    </form>
  );
}
