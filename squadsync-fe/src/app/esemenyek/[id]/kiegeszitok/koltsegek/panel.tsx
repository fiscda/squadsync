import {
  getKoltsegek,
  type KiadasAllas,
  type Koltsegek,
} from "@/lib/kiegeszito/koltsegek";
import { secondaryButtonClass } from "@/lib/ui";
import type { KiegeszitoPanelProps } from "../panel-props";
import {
  addKiadasAction,
  deleteKiadasAction,
  markKifizetveAction,
  updateKiadasAction,
} from "./actions";
import { ElszamolasSection } from "./elszamolas";
import { formatFt, formatNetto } from "./ft";
import { KiadasForm, type KiadasFormPerson } from "./kiadas-form";

const inputClass =
  "rounded-lg border border-zinc-300 bg-transparent px-3 py-1.5 text-sm dark:border-zinc-700";

/** Költségek: the Kiadások, their splits and everyone's balance. */
export async function KoltsegekPanel({
  esemeny,
  felhasznaloId,
}: KiegeszitoPanelProps) {
  const koltsegek = await getKoltsegek(esemeny.id);
  if (koltsegek === null) {
    return null;
  }
  const { kiadasok, canAdd } = koltsegek;
  const resztvevok: KiadasFormPerson[] = esemeny.resztvevok.map((r) => ({
    id: r.felhasznaloId,
    name: r.name,
  }));
  const isResztvevo = resztvevok.some((p) => p.id === felhasznaloId);
  const locked =
    koltsegek.elszamolas?.allapot === "kiszamolva" ||
    koltsegek.elszamolas?.allapot === "elszamolva";

  return (
    <div className="flex flex-col gap-4">
      {koltsegek.elszamolas?.allapot === "elszamolva" ? (
        <p className="font-medium text-green-700 dark:text-green-400">
          Elszámolva ✓
        </p>
      ) : null}

      {kiadasok.length === 0 ? (
        <p className="text-sm text-zinc-500">Még nincs egy kiadás sem.</p>
      ) : (
        <ul className="flex flex-col gap-3">
          {kiadasok.map((kiadas) => (
            <KiadasItem
              key={kiadas.id}
              esemenyId={esemeny.id}
              kiadas={kiadas}
              resztvevok={resztvevok}
            />
          ))}
        </ul>
      )}

      <Egyenlegek koltsegek={koltsegek} />

      <ElszamolasSection esemenyId={esemeny.id} koltsegek={koltsegek} />

      {canAdd ? (
        <details className="rounded-xl border border-zinc-200 p-3 dark:border-zinc-800">
          <summary className="cursor-pointer text-sm font-medium">
            Új kiadás
          </summary>
          <div className="mt-3">
            <KiadasForm
              action={addKiadasAction.bind(null, esemeny.id)}
              people={resztvevok}
              defaults={{
                description: "",
                amount: null,
                payerId: isResztvevo
                  ? felhasznaloId
                  : (resztvevok[0]?.id ?? ""),
                allapot: "kifizetve",
                reszek: Object.fromEntries(resztvevok.map((p) => [p.id, 0])),
                egyedi: false,
              }}
              submitLabel="Hozzáadás"
            />
          </div>
        </details>
      ) : (
        <p className="text-sm text-zinc-600 dark:text-zinc-400">
          {locked
            ? "Az elszámolás elkészült, ezért a kiadások le vannak zárva."
            : esemeny.allapot === "egyeztetes"
              ? "Kiadást a véglegesítés után lehet rögzíteni."
              : esemeny.allapot === "elmaradt"
                ? "Elmaradt eseménynél már nem lehet kiadást rögzíteni."
                : "Kiadást a résztvevők rögzíthetnek."}
        </p>
      )}
    </div>
  );
}

function KiadasItem({
  esemenyId,
  kiadas,
  resztvevok,
}: {
  esemenyId: string;
  kiadas: KiadasAllas;
  resztvevok: KiadasFormPerson[];
}) {
  const tervezett = kiadas.allapot === "tervezett";
  // People already in it stay editable even if they dropped out.
  const people = [...resztvevok];
  for (const p of [
    { id: kiadas.payerId, name: kiadas.payerName },
    ...kiadas.reszek.map((r) => ({ id: r.felhasznaloId, name: r.name })),
  ]) {
    if (p.id !== "" && !people.some((q) => q.id === p.id)) {
      people.push(p);
    }
  }

  return (
    <li className="flex flex-col gap-2 border-t border-zinc-200 pt-3 first:border-t-0 first:pt-0 dark:border-zinc-800">
      <div className="flex items-start justify-between gap-4">
        <span className="flex flex-wrap items-center gap-2">
          <span className="font-medium">{kiadas.description}</span>
          <span
            className={`rounded-full px-2 py-0.5 text-xs font-medium ${
              tervezett
                ? "bg-amber-100 text-amber-900 dark:bg-amber-950 dark:text-amber-200"
                : "bg-green-100 text-green-900 dark:bg-green-950 dark:text-green-200"
            }`}
          >
            {tervezett ? "Tervezett" : "Kifizetve"}
          </span>
        </span>
        <span className="shrink-0 text-sm font-medium">
          {tervezett ? "kb. " : ""}
          {formatFt(kiadas.amount)}
        </span>
      </div>
      <p className="text-sm text-zinc-600 dark:text-zinc-400">
        {tervezett ? "Fizeti" : "Fizette"}: {kiadas.payerName} ·{" "}
        {kiadas.egyenlo ? "egyenlően" : "egyedi összegekkel"}:{" "}
        {kiadas.reszek.map((r) => `${r.name} ${formatFt(r.amount)}`).join(", ")}
      </p>

      {kiadas.canEdit && tervezett ? (
        <form
          action={markKifizetveAction.bind(null, esemenyId, kiadas.id)}
          className="flex flex-wrap items-center gap-2 text-sm"
        >
          {kiadas.egyenlo ? (
            <>
              <input
                name="amount"
                type="number"
                required
                min={1}
                max={100000000}
                step={1}
                defaultValue={kiadas.amount}
                aria-label={`${kiadas.description}: a valódi összeg (Ft)`}
                className={`${inputClass} w-32`}
              />
              <button type="submit" className={secondaryButtonClass}>
                Kifizetve
              </button>
            </>
          ) : (
            <>
              {/* A custom split only adds up to its current amount. */}
              <input type="hidden" name="amount" value={kiadas.amount} />
              <button type="submit" className={secondaryButtonClass}>
                Kifizetve ({formatFt(kiadas.amount)})
              </button>
              <span className="text-xs text-zinc-500">
                Más összeghez nyisd meg a Szerkesztést, és add meg újra a
                részeket.
              </span>
            </>
          )}
        </form>
      ) : null}

      {kiadas.canEdit ? (
        <details className="text-sm">
          <summary className="cursor-pointer text-zinc-500">
            Szerkesztés
          </summary>
          <div className="mt-2 flex flex-col gap-3">
            <KiadasForm
              action={updateKiadasAction.bind(null, esemenyId, kiadas.id)}
              people={people}
              defaults={{
                description: kiadas.description,
                amount: kiadas.amount,
                payerId: kiadas.payerId,
                allapot: kiadas.allapot,
                reszek: Object.fromEntries(
                  kiadas.reszek.map((r) => [r.felhasznaloId, r.amount]),
                ),
                egyedi: !kiadas.egyenlo,
              }}
              submitLabel="Mentés"
            />
            <form action={deleteKiadasAction.bind(null, esemenyId, kiadas.id)}>
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

function Egyenlegek({ koltsegek }: { koltsegek: Koltsegek }) {
  const { egyenlegek, kiadasok, tervezett } = koltsegek;
  const planned = kiadasok.filter((k) => k.allapot === "tervezett");
  if (egyenlegek.length === 0 && tervezett === 0) {
    return null;
  }
  return (
    <div className="flex flex-col gap-2">
      {egyenlegek.length > 0 ? (
        <table className="w-full text-sm">
          <caption className="mb-1 text-left font-medium">
            Ki hogy áll (csak a kifizetett kiadások)
          </caption>
          <thead>
            <tr className="text-left text-zinc-500">
              <th className="py-1 font-normal">Név</th>
              <th className="py-1 text-right font-normal">Fizetett</th>
              <th className="py-1 text-right font-normal">Része</th>
              <th className="py-1 text-right font-normal">Egyenleg</th>
            </tr>
          </thead>
          <tbody>
            {egyenlegek.map((e) => (
              <tr
                key={e.felhasznaloId}
                className="border-t border-zinc-200 dark:border-zinc-800"
              >
                <td className="py-1">{e.name}</td>
                <td className="py-1 text-right">{formatFt(e.fizetett)}</td>
                <td className="py-1 text-right">{formatFt(e.resz)}</td>
                <td
                  className={`py-1 text-right font-medium ${
                    e.netto > 0
                      ? "text-green-700 dark:text-green-400"
                      : e.netto < 0
                        ? "text-red-700 dark:text-red-400"
                        : ""
                  }`}
                >
                  {formatNetto(e.netto)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : null}
      {planned.length > 0 ? (
        <p className="rounded-xl bg-amber-50 p-3 text-sm text-amber-900 dark:bg-amber-950 dark:text-amber-200">
          Még tervezett, nem számít bele:{" "}
          {planned
            .map(
              (k) =>
                `${k.description} (${k.payerName}, kb. ${formatFt(k.amount)})`,
            )
            .join(", ")}
        </p>
      ) : null}
    </div>
  );
}
