import { getFelszereles, type TetelAllas } from "@/lib/kiegeszito/felszereles";
import { secondaryButtonClass } from "@/lib/ui";
import type { KiegeszitoPanelProps } from "../panel-props";
import {
  addTetelAction,
  deleteTetelAction,
  setVallalasAction,
  updateTetelAction,
} from "./actions";

const inputClass =
  "rounded-lg border border-zinc-300 bg-transparent px-3 py-1.5 text-sm dark:border-zinc-700";

/** Felszerelés: the Tételek to bring, their claims and what is missing. */
export async function FelszerelesPanel({ esemeny }: KiegeszitoPanelProps) {
  const felszereles = await getFelszereles(esemeny.id);
  if (felszereles === null) {
    return null;
  }
  const { tetelek, hianyzik, emlekezteto, canAdd, canClaim } = felszereles;
  const claimable = tetelek.some((t) => !t.mindenkiHozza);

  return (
    <div className="flex flex-col gap-4">
      {emlekezteto.length > 0 ? (
        <p className="rounded-xl bg-amber-50 p-3 text-sm text-amber-900 dark:bg-amber-950 dark:text-amber-200">
          Ne felejtsd el magaddal hozni: {emlekezteto.join(", ")}
        </p>
      ) : null}

      {tetelek.length === 0 ? (
        <p className="text-sm text-zinc-500">Még nincs egy tétel sem.</p>
      ) : (
        <>
          {claimable ? (
            <p className="text-sm text-zinc-600 dark:text-zinc-400">
              {hianyzik > 0
                ? `Még ${hianyzik} db hiányzik.`
                : "Minden megvan. ✓"}
              {canClaim
                ? ""
                : esemeny.allapot === "veglegesitve"
                  ? " Vállalni a résztvevők tudnak."
                  : " Vállalni a véglegesítés után lehet."}
            </p>
          ) : null}
          <ul className="flex flex-col gap-3">
            {tetelek.map((tetel) => (
              <TetelItem
                key={tetel.id}
                esemenyId={esemeny.id}
                tetel={tetel}
                canClaim={canClaim}
              />
            ))}
          </ul>
        </>
      )}

      {canAdd ? (
        <form
          action={addTetelAction.bind(null, esemeny.id)}
          className="flex flex-wrap items-end gap-2"
        >
          <label className="flex flex-col gap-1 text-sm">
            Új tétel
            <input
              name="name"
              required
              maxLength={100}
              placeholder="pl. sátor"
              className={inputClass}
            />
          </label>
          <label className="flex flex-col gap-1 text-sm">
            Darab
            <input
              name="quantity"
              type="number"
              min={1}
              max={999}
              defaultValue={1}
              className={`${inputClass} w-20`}
            />
          </label>
          <label className="flex items-center gap-2 py-1.5 text-sm">
            <input name="mindenkiHozza" type="checkbox" />
            Mindenki hozza
          </label>
          <button type="submit" className={secondaryButtonClass}>
            Hozzáadás
          </button>
        </form>
      ) : null}
    </div>
  );
}

function TetelItem({
  esemenyId,
  tetel,
  canClaim,
}: {
  esemenyId: string;
  tetel: TetelAllas;
  canClaim: boolean;
}) {
  const maxClaim = tetel.hianyzik + tetel.sajat;
  return (
    <li className="flex flex-col gap-2 border-t border-zinc-200 pt-3 first:border-t-0 first:pt-0 dark:border-zinc-800">
      <div className="flex items-start justify-between gap-4">
        <span className="font-medium">{tetel.name}</span>
        <span className="shrink-0 text-sm text-zinc-600 dark:text-zinc-400">
          {tetel.mindenkiHozza
            ? "Mindenki hozza"
            : `${tetel.vallalt}/${tetel.quantity} megvan${
                tetel.hianyzik > 0 ? ` · ${tetel.hianyzik} hiányzik` : " ✓"
              }`}
        </span>
      </div>

      {tetel.vallalasok.length > 0 ? (
        <p className="text-sm">
          {tetel.vallalasok
            .map((v) => `${v.name}: ${v.quantity} db`)
            .join(", ")}
        </p>
      ) : null}

      {canClaim && !tetel.mindenkiHozza && maxClaim > 0 ? (
        <div className="flex flex-wrap items-center gap-2">
          <form
            action={setVallalasAction.bind(null, esemenyId, tetel.id)}
            className="flex items-center gap-2"
          >
            <input
              name="quantity"
              type="number"
              min={1}
              max={maxClaim}
              defaultValue={tetel.sajat > 0 ? tetel.sajat : maxClaim}
              aria-label={`${tetel.name}: hány darabot hozol`}
              className={`${inputClass} w-20`}
            />
            <button type="submit" className={secondaryButtonClass}>
              {tetel.sajat > 0 ? "Módosítom" : "Vállalom"}
            </button>
          </form>
          {tetel.sajat > 0 ? (
            <form action={setVallalasAction.bind(null, esemenyId, tetel.id)}>
              <input type="hidden" name="quantity" value="0" />
              <button type="submit" className={secondaryButtonClass}>
                Visszavonom
              </button>
            </form>
          ) : null}
        </div>
      ) : null}

      {tetel.canEdit ? (
        <details className="text-sm">
          <summary className="cursor-pointer text-zinc-500">
            Szerkesztés
          </summary>
          <div className="mt-2 flex flex-wrap items-end gap-2">
            <form
              action={updateTetelAction.bind(null, esemenyId, tetel.id)}
              className="flex flex-wrap items-end gap-2"
            >
              <input
                name="name"
                required
                maxLength={100}
                defaultValue={tetel.name}
                aria-label="Név"
                className={inputClass}
              />
              <input
                name="quantity"
                type="number"
                min={Math.max(1, tetel.vallalt)}
                max={999}
                defaultValue={tetel.quantity}
                aria-label="Darab"
                className={`${inputClass} w-20`}
              />
              <label className="flex items-center gap-2 py-1.5">
                <input
                  name="mindenkiHozza"
                  type="checkbox"
                  defaultChecked={tetel.mindenkiHozza}
                />
                Mindenki hozza
              </label>
              <button type="submit" className={secondaryButtonClass}>
                Mentés
              </button>
            </form>
            <form action={deleteTetelAction.bind(null, esemenyId, tetel.id)}>
              <button
                type="submit"
                className={`${secondaryButtonClass} text-red-700 dark:text-red-400`}
              >
                Törlés
              </button>
            </form>
          </div>
        </details>
      ) : null}
    </li>
  );
}
