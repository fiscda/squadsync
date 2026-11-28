import type {
  ElszamolasSzemely,
  Koltsegek,
  Utalas,
} from "@/lib/kiegeszito/koltsegek";
import { buttonClass, formatDate, secondaryButtonClass } from "@/lib/ui";
import {
  closeElszamolasAction,
  confirmKiadasokAction,
  reopenElszamolasAction,
  setUtalasMegvoltAction,
  unconfirmKiadasokAction,
} from "./actions";
import { formatFt } from "./ft";

const warningClass =
  "rounded-xl bg-amber-50 p-3 text-sm text-amber-900 dark:bg-amber-950 dark:text-amber-200";

function nameList(list: ElszamolasSzemely[]): string {
  return list.map((p) => (p.name === "" ? "Vendég" : p.name)).join(", ");
}

/**
 * The Elszámolás inside the Költségek panel: who has confirmed, the
 * Szervező's close-early step, the Utalások with their megvolt toggles, and
 * reopening.
 */
export function ElszamolasSection({
  esemenyId,
  koltsegek,
}: {
  esemenyId: string;
  koltsegek: Koltsegek;
}) {
  const e = koltsegek.elszamolas;
  if (e === null) {
    return null;
  }
  const planned = koltsegek.kiadasok.filter((k) => k.allapot === "tervezett");
  const plannedText = planned
    .map((k) => `${k.description} (${k.payerName}, kb. ${formatFt(k.amount)})`)
    .join(", ");

  return (
    <section
      aria-labelledby="elszamolas-cim"
      className="flex flex-col gap-3 rounded-xl border border-zinc-200 p-3 dark:border-zinc-800"
    >
      <h3 id="elszamolas-cim" className="font-medium">
        Elszámolás
      </h3>

      {e.allapot === "lezajlas_elott" ? (
        <p className="text-sm text-zinc-600 dark:text-zinc-400">
          Az elszámolás az esemény lezajlása után nyílik meg. Akkor mindenki
          jelzi, hogy rögzítette a kiadásait, és kiszámoljuk, ki kinek mennyit
          utaljon.
        </p>
      ) : null}

      {e.allapot === "megerosites" ? (
        <>
          <p className="text-sm text-zinc-600 dark:text-zinc-400">
            Ha mindenki jelezte, hogy rögzítette a kiadásait, az elszámolás
            magától elkészül.
          </p>
          <ul className="text-sm">
            <li>
              Rögzítette:{" "}
              {e.megerositettek.length > 0
                ? nameList(e.megerositettek)
                : "még senki"}
            </li>
            <li>
              Még nem jelezte:{" "}
              {e.hianyzok.length > 0 ? nameList(e.hianyzok) : "senki"}
            </li>
          </ul>

          {e.megerositette ? (
            <form
              action={unconfirmKiadasokAction.bind(null, esemenyId)}
              className="flex flex-wrap items-center gap-2 text-sm"
            >
              <span>✓ Jelezted, hogy rögzítetted a kiadásaidat.</span>
              {e.canUnconfirm ? (
                <button type="submit" className={secondaryButtonClass}>
                  Mégsem
                </button>
              ) : null}
            </form>
          ) : e.canConfirm ? (
            e.tervezettFizeto.length > 0 ? (
              <div className="flex flex-col gap-2">
                <p className={warningClass}>
                  Amíg te fizeted ezeket a tervezett kiadásokat, nem jelezheted,
                  hogy rögzítetted a kiadásaidat: {e.tervezettFizeto.join(", ")}
                  . Jelöld őket kifizetettnek a valódi összeggel, vagy töröld
                  őket.
                </p>
                <button
                  type="button"
                  disabled
                  className={`${buttonClass} self-start opacity-50`}
                >
                  Rögzítettem a kiadásaimat
                </button>
              </div>
            ) : (
              <form action={confirmKiadasokAction.bind(null, esemenyId)}>
                <button type="submit" className={buttonClass}>
                  Rögzítettem a kiadásaimat
                </button>
              </form>
            )
          ) : null}

          {e.canClose ? (
            <details className="text-sm">
              <summary className="cursor-pointer text-zinc-500">
                Lezárás most, a hiányzók nélkül
              </summary>
              <form
                action={closeElszamolasAction.bind(null, esemenyId)}
                className="mt-2 flex flex-col gap-2"
              >
                <p>
                  {e.hianyzok.length > 0
                    ? `Még nem jelezte: ${nameList(e.hianyzok)}. Ha most lezárod, az elszámolás az ő esetleges további kiadásaik nélkül készül el.`
                    : "Mindenki jelezte, hogy rögzítette a kiadásait."}{" "}
                  Utána a kiadások le lesznek zárva.
                </p>
                {planned.length > 0 ? (
                  <p className={warningClass}>
                    Ezek a tervezett kiadások nem számítanak bele: {plannedText}
                    .
                  </p>
                ) : null}
                <button
                  type="submit"
                  className={`${secondaryButtonClass} self-start`}
                >
                  Lezárom és kiszámolom
                </button>
              </form>
            </details>
          ) : null}
        </>
      ) : null}

      {e.allapot === "kiszamolva" || e.allapot === "elszamolva" ? (
        <>
          <p className="text-sm text-zinc-600 dark:text-zinc-400">
            Elkészült
            {e.kiszamolvaAt ? ` ${formatDate(e.kiszamolvaAt)}` : ""}
            {e.lezartaName !== ""
              ? `: ${e.lezartaName} szervezőként lezárta, nem várt meg mindenkit.`
              : ", miután mindenki jelezte, hogy rögzítette a kiadásait."}{" "}
            A kiadások le vannak zárva.
          </p>
          {planned.length > 0 ? (
            <p className={warningClass}>
              Ezek a tervezett kiadások nem számítottak bele: {plannedText}.
            </p>
          ) : null}
          {e.utalasok.length === 0 ? (
            <p className="text-sm">
              Senkinek nem kell utalnia: mindenki rendben van.
            </p>
          ) : (
            <>
              <ul className="flex flex-col gap-2">
                {e.utalasok.map((u) => (
                  <UtalasItem key={u.id} esemenyId={esemenyId} utalas={u} />
                ))}
              </ul>
              <p className="text-sm text-zinc-500">
                Összesen{" "}
                {formatFt(e.utalasok.reduce((s, u) => s + u.amount, 0))}{" "}
                {e.utalasok.length} utalásban ·{" "}
                {e.utalasok.filter((u) => u.megvoltAt !== null).length} megvolt
              </p>
            </>
          )}
          {e.canReopen ? (
            <details className="text-sm">
              <summary className="cursor-pointer text-zinc-500">
                Újranyitás
              </summary>
              <form
                action={reopenElszamolasAction.bind(null, esemenyId)}
                className="mt-2 flex flex-col gap-2"
              >
                <p>
                  Az utalások törlődnek, a kiadások újra szerkeszthetők, és
                  mindenkinek újra jeleznie kell, hogy rögzítette a kiadásait.
                  Csak addig lehet, amíg egy utalás sincs megvoltnak jelölve.
                </p>
                <button
                  type="submit"
                  className={`${secondaryButtonClass} self-start`}
                >
                  Újranyitom az elszámolást
                </button>
              </form>
            </details>
          ) : null}
        </>
      ) : null}
    </section>
  );
}

function UtalasItem({
  esemenyId,
  utalas,
}: {
  esemenyId: string;
  utalas: Utalas;
}) {
  const megvolt = utalas.megvoltAt !== null;
  return (
    <li className="flex flex-wrap items-center justify-between gap-2 text-sm">
      <span>
        <span className="font-medium">{utalas.fromName}</span> →{" "}
        <span className="font-medium">{utalas.toName}</span>:{" "}
        {formatFt(utalas.amount)}
        {megvolt ? (
          <span className="ml-2 text-green-700 dark:text-green-400">
            megvolt ✓
            {utalas.megvoltByName !== "" ? ` (${utalas.megvoltByName})` : ""}
          </span>
        ) : null}
      </span>
      {utalas.canMark ? (
        <form
          action={setUtalasMegvoltAction.bind(
            null,
            esemenyId,
            utalas.id,
            !megvolt,
          )}
        >
          <button type="submit" className={secondaryButtonClass}>
            {megvolt ? "Mégsem" : "Megvolt"}
          </button>
        </form>
      ) : null}
    </li>
  );
}
