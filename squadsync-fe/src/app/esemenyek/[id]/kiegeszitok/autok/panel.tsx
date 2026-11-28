import type { Resztvevo } from "@/lib/backend";
import { type AutoAllas, getAutok, type Szemely } from "@/lib/kiegeszito/autok";
import { secondaryButtonClass } from "@/lib/ui";
import {
  type KiegeszitoPanelProps,
  sajatVendegResztvevok,
} from "../panel-props";
import {
  addUzemanyagAction,
  claimUlesAction,
  deleteAutoAction,
  moveUtasAction,
  offerAutoAction,
  releaseUlesAction,
  setSajatUtonAction,
  updateAutoAction,
} from "./actions";
import { VendegUlesek } from "./vendeg-ulesek";

const inputClass =
  "rounded-lg border border-zinc-300 bg-transparent px-3 py-1.5 text-sm dark:border-zinc-700";

const dangerClass = `${secondaryButtonClass} text-red-700 dark:text-red-400`;

function nameOf(s: Szemely): string {
  return s.name === "" ? "Vendég" : s.name;
}

/** Autók: who drives, who sits where, and who still has no seat. */
export async function AutokPanel({
  esemeny,
  felhasznaloId,
}: KiegeszitoPanelProps) {
  const autok = await getAutok(esemeny.id);
  if (autok === null) {
    return null;
  }
  const {
    helyNelkul,
    sajatUton,
    canClaim,
    sajatUlesAutoId,
    sajatUtonVan,
    isSzervezo,
  } = autok;
  // Go refuses every change but "Beszállok" and the like unless the Esemény
  // is véglegesítve, so the controls that only change seats show only then.
  const veglegesitve = esemeny.allapot === "veglegesitve";
  const canMove = isSzervezo && veglegesitve;
  // The fuel shortcut records a Költségek Kiadás, only while Résztvevők exist.
  const hasResztvevok =
    esemeny.allapot === "veglegesitve" || esemeny.allapot === "lezajlott";
  const koltsegekOn = esemeny.kiegeszitok.some(
    (k) => k.kind === "koltsegek" && k.enabled,
  );
  const canUzemanyag =
    hasResztvevok &&
    esemeny.resztvevok.some((r) => r.felhasznaloId === felhasznaloId);
  const anySetup = autok.autok.length > 0 || sajatUton.length > 0;

  return (
    <div className="flex flex-col gap-4">
      {anySetup ? (
        <p className="text-sm text-zinc-600 dark:text-zinc-400">
          {helyNelkul.length > 0
            ? `🚗 ${helyNelkul.length} főnek nincs helye: ${helyNelkul
                .map(nameOf)
                .join(", ")}`
            : esemeny.allapot === "veglegesitve"
              ? "🚗 Mindenkinek van helye. ✓"
              : "🚗 A helyek a véglegesítés után számítanak."}
        </p>
      ) : (
        <p className="text-sm text-zinc-500">
          Még nincs autó.
          {esemeny.allapot === "veglegesitve"
            ? ""
            : " Autót a véglegesítés után lehet ajánlani."}
        </p>
      )}

      {autok.autok.length > 0 ? (
        <ul className="flex flex-col gap-3">
          {autok.autok.map((auto) => (
            <AutoItem
              key={auto.id}
              esemenyId={esemeny.id}
              auto={auto}
              all={autok.autok}
              canClaim={canClaim}
              hasSeat={sajatUlesAutoId !== ""}
              mine={sajatUlesAutoId === auto.id && veglegesitve}
              canMove={canMove}
              uzemanyag={
                canUzemanyag
                  ? {
                      koltsegekOn,
                      resztvevok: esemeny.resztvevok,
                      felhasznaloId,
                    }
                  : null
              }
            />
          ))}
        </ul>
      ) : null}

      {sajatUton.length > 0 ? (
        <p className="text-sm">
          Saját úton: {sajatUton.map(nameOf).join(", ")}
        </p>
      ) : null}

      {canClaim ? (
        <form action={setSajatUtonAction.bind(null, esemeny.id, !sajatUtonVan)}>
          <button type="submit" className={secondaryButtonClass}>
            {sajatUtonVan ? "Mégsem megyek saját úton" : "Saját úton megyek"}
          </button>
        </form>
      ) : null}

      {canMove && autok.autok.length > 0 ? (
        <ul className="flex flex-col gap-2 text-sm">
          {[
            ...helyNelkul.map((s) => ({ szemely: s, sajatUton: false })),
            ...sajatUton.map((s) => ({ szemely: s, sajatUton: true })),
          ]
            .filter(({ szemely }) => szemely.felhasznaloId !== "")
            .map(({ szemely: s, sajatUton: uton }) => (
              <li key={s.felhasznaloId} className="flex items-center gap-2">
                <span>
                  {nameOf(s)}
                  {uton ? " (saját úton)" : ""}
                </span>
                <MoveForm
                  esemenyId={esemeny.id}
                  szemely={s}
                  autok={autok.autok}
                  currentAutoId=""
                  label="Beültet"
                />
              </li>
            ))}
        </ul>
      ) : null}

      {canClaim ? (
        <form
          action={offerAutoAction.bind(null, esemeny.id)}
          className="flex flex-wrap items-end gap-2"
        >
          <label className="flex flex-col gap-1 text-sm">
            Szabad helyek (a sofőrön kívül)
            <input
              name="seats"
              type="number"
              required
              min={1}
              max={8}
              defaultValue={3}
              className={`${inputClass} w-24`}
            />
          </label>
          <label className="flex flex-col gap-1 text-sm">
            Megjegyzés
            <input
              name="note"
              maxLength={100}
              placeholder="pl. indulás 8:00, Keleti"
              className={inputClass}
            />
          </label>
          <button type="submit" className={secondaryButtonClass}>
            Autót ajánlok
          </button>
        </form>
      ) : null}

      <VendegUlesek
        esemenyId={esemeny.id}
        autok={autok}
        vendegek={sajatVendegResztvevok(esemeny, felhasznaloId)}
      />
    </div>
  );
}

function MoveForm({
  esemenyId,
  szemely,
  autok,
  currentAutoId,
  label,
}: {
  esemenyId: string;
  szemely: Szemely;
  autok: AutoAllas[];
  currentAutoId: string;
  label: string;
}) {
  const targets = autok.filter((a) => a.id !== currentAutoId && a.szabad > 0);
  if (targets.length === 0) {
    return null;
  }
  return (
    <form
      action={moveUtasAction.bind(null, esemenyId, szemely.felhasznaloId)}
      className="flex items-center gap-2"
    >
      <select
        name="autoId"
        aria-label={`${nameOf(szemely)}: melyik autóba`}
        className={inputClass}
      >
        {targets.map((a) => (
          <option key={a.id} value={a.id}>
            {a.soforName} autója ({a.szabad} szabad)
          </option>
        ))}
      </select>
      <button type="submit" className={secondaryButtonClass}>
        {label}
      </button>
    </form>
  );
}

function AutoItem({
  esemenyId,
  auto,
  all,
  canClaim,
  hasSeat,
  mine,
  canMove,
  uzemanyag,
}: {
  esemenyId: string;
  auto: AutoAllas;
  all: AutoAllas[];
  canClaim: boolean;
  hasSeat: boolean;
  mine: boolean;
  canMove: boolean;
  uzemanyag: {
    koltsegekOn: boolean;
    resztvevok: Resztvevo[];
    felhasznaloId: string;
  } | null;
}) {
  return (
    <li className="flex flex-col gap-2 border-t border-zinc-200 pt-3 first:border-t-0 first:pt-0 dark:border-zinc-800">
      <div className="flex items-start justify-between gap-4">
        <span className="font-medium">{auto.soforName} autója</span>
        <span className="shrink-0 text-sm text-zinc-600 dark:text-zinc-400">
          {auto.utasok.length}/{auto.seats} hely foglalt
          {auto.szabad === 0 ? " ✓" : ""}
        </span>
      </div>
      {auto.note !== "" ? (
        <p className="text-sm text-zinc-600 dark:text-zinc-400">{auto.note}</p>
      ) : null}

      {auto.utasok.length > 0 ? (
        <ul className="flex flex-col gap-1 text-sm">
          {auto.utasok.map((u) => (
            <li
              key={u.felhasznaloId || u.vendegId}
              className="flex flex-wrap items-center gap-2"
            >
              <span>{nameOf(u)}</span>
              {canMove && u.felhasznaloId !== "" ? (
                <>
                  <MoveForm
                    esemenyId={esemenyId}
                    szemely={u}
                    autok={all}
                    currentAutoId={auto.id}
                    label="Áthelyez"
                  />
                  <form
                    action={releaseUlesAction.bind(
                      null,
                      esemenyId,
                      u.felhasznaloId,
                    )}
                  >
                    <button type="submit" className={dangerClass}>
                      Kivesz
                    </button>
                  </form>
                </>
              ) : null}
            </li>
          ))}
        </ul>
      ) : null}

      {canClaim && !hasSeat && auto.szabad > 0 ? (
        <form action={claimUlesAction.bind(null, esemenyId, auto.id)}>
          <button type="submit" className={secondaryButtonClass}>
            Beszállok
          </button>
        </form>
      ) : null}
      {mine ? (
        <form action={releaseUlesAction.bind(null, esemenyId, "")}>
          <button type="submit" className={secondaryButtonClass}>
            Kiszállok
          </button>
        </form>
      ) : null}

      {uzemanyag !== null ? (
        <UzemanyagForm esemenyId={esemenyId} auto={auto} {...uzemanyag} />
      ) : null}

      {auto.canEdit || auto.canDelete ? (
        <details className="text-sm">
          <summary className="cursor-pointer text-zinc-500">
            {auto.canEdit ? "Szerkesztés" : "Törlés"}
          </summary>
          <div className="mt-2 flex flex-wrap items-end gap-2">
            {auto.canEdit ? (
              <form
                action={updateAutoAction.bind(null, esemenyId, auto.id)}
                className="flex flex-wrap items-end gap-2"
              >
                <input
                  name="seats"
                  type="number"
                  required
                  min={Math.max(1, auto.utasok.length)}
                  max={8}
                  defaultValue={auto.seats}
                  aria-label="Szabad helyek"
                  className={`${inputClass} w-24`}
                />
                <input
                  name="note"
                  maxLength={100}
                  defaultValue={auto.note}
                  aria-label="Megjegyzés"
                  className={inputClass}
                />
                <button type="submit" className={secondaryButtonClass}>
                  Mentés
                </button>
              </form>
            ) : null}
            {auto.canDelete ? (
              <form action={deleteAutoAction.bind(null, esemenyId, auto.id)}>
                <button type="submit" className={dangerClass}>
                  Autó törlése
                </button>
              </form>
            ) : null}
          </div>
        </details>
      ) : null}
    </li>
  );
}

/**
 * "⛽ Üzemanyag": a fuel Kiadás from the Autó, paid by anyone (the Sofőr by
 * default), split among the Autó's people or every Résztvevő.
 */
function UzemanyagForm({
  esemenyId,
  auto,
  koltsegekOn,
  resztvevok,
  felhasznaloId,
}: {
  esemenyId: string;
  auto: AutoAllas;
  koltsegekOn: boolean;
  resztvevok: Resztvevo[];
  felhasznaloId: string;
}) {
  const payerDefault = resztvevok.some((r) => r.felhasznaloId === auto.soforId)
    ? auto.soforId
    : felhasznaloId;
  return (
    <details className="text-sm">
      <summary className="cursor-pointer text-zinc-500">⛽ Üzemanyag</summary>
      {koltsegekOn ? (
        <form
          action={addUzemanyagAction.bind(null, esemenyId, auto.id)}
          className="mt-2 flex flex-col gap-3"
        >
          <div className="flex flex-wrap items-end gap-2">
            <label className="flex flex-col gap-1">
              Összeg (Ft)
              <input
                name="amount"
                type="number"
                required
                min={1}
                step={1}
                className={`${inputClass} w-32`}
              />
            </label>
            <label className="flex flex-col gap-1">
              Leírás
              <input
                name="description"
                maxLength={200}
                placeholder={`Üzemanyag – ${auto.soforName} autója`}
                className={inputClass}
              />
            </label>
            <label className="flex flex-col gap-1">
              Ki fizette
              <select
                name="payerId"
                defaultValue={payerDefault}
                className={inputClass}
              >
                {resztvevok
                  .filter((r) => r.felhasznaloId !== "")
                  .map((r) => (
                    <option key={r.felhasznaloId} value={r.felhasznaloId}>
                      {r.name}
                    </option>
                  ))}
              </select>
            </label>
          </div>
          <fieldset className="flex flex-col gap-1">
            <legend className="mb-1">Kik között osszuk el?</legend>
            <label className="flex items-center gap-2">
              <input type="radio" name="split" value="auto" defaultChecked />
              Az autó utasai ({auto.utasok.length + 1} fő)
            </label>
            <label className="flex items-center gap-2">
              <input type="radio" name="split" value="mindenki" />
              Minden résztvevő
            </label>
          </fieldset>
          <div>
            <button type="submit" className={secondaryButtonClass}>
              Üzemanyag rögzítése
            </button>
          </div>
        </form>
      ) : (
        <p className="mt-2 text-zinc-600 dark:text-zinc-400">
          Az üzemanyag rögzítéséhez a Költségek kiegészítő kell. Kérd meg a
          szervezőt, hogy kapcsolja be.
        </p>
      )}
    </details>
  );
}
