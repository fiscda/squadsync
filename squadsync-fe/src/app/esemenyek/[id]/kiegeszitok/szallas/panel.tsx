import {
  type AgyAllas,
  getSzallas,
  type Szemely,
  type SzobaAllas,
} from "@/lib/kiegeszito/szallas";
import { secondaryButtonClass } from "@/lib/ui";
import {
  type KiegeszitoPanelProps,
  sajatVendegResztvevok,
} from "../panel-props";
import {
  addAgyAction,
  addSzobaAction,
  claimAgyAction,
  deleteAgyAction,
  deleteSzobaAction,
  moveAlvoAction,
  releaseFoglalasAction,
  renameSzobaAction,
  setMasholAlszikAction,
  setSzallasNoteAction,
  updateAgyAction,
} from "./actions";
import { ConfirmDelete } from "./confirm-delete";
import { VendegAgyak } from "./vendeg-agyak";

const inputClass =
  "rounded-lg border border-zinc-300 bg-transparent px-3 py-1.5 text-sm dark:border-zinc-700";

const dangerClass = `${secondaryButtonClass} text-red-700 dark:text-red-400`;

function nameOf(s: Szemely): string {
  return s.name === "" ? "Vendég" : s.name;
}

function szemelyKey(s: Szemely): string {
  return s.felhasznaloId || s.vendegId || "";
}

/** Szállás: the Szobák and Ágyak, who sleeps where, and who has no bed. */
export async function SzallasPanel({
  esemeny,
  felhasznaloId,
}: KiegeszitoPanelProps) {
  const szallas = await getSzallas(esemeny.id);
  if (szallas === null) {
    return null;
  }
  const { agyNelkul, masholAlszik, canSetup, canClaim, isSzervezo } = szallas;
  // Go refuses every spot change unless the Esemény is véglegesítve, so
  // the controls that only change spots show only then.
  const veglegesitve = esemeny.allapot === "veglegesitve";
  const canMove = isSzervezo && veglegesitve;
  const anySetup = szallas.szobak.length > 0 || masholAlszik.length > 0;
  const allAgyak = szallas.szobak.flatMap((sz) => sz.agyak);

  return (
    <div className="flex flex-col gap-4">
      {szallas.note !== "" ? (
        <p className="text-sm whitespace-pre-line">{szallas.note}</p>
      ) : null}
      {canSetup ? (
        <details className="text-sm">
          <summary className="cursor-pointer text-zinc-500">
            {szallas.note === ""
              ? "Szállás megnevezése"
              : "Megnevezés módosítása"}
          </summary>
          <form
            action={setSzallasNoteAction.bind(null, esemeny.id)}
            className="mt-2 flex flex-wrap items-end gap-2"
          >
            <input
              name="note"
              maxLength={200}
              defaultValue={szallas.note}
              placeholder="pl. Napfény Vendégház, Balatonfüred"
              aria-label="A szállás neve vagy megjegyzés"
              className={`${inputClass} w-80 max-w-full`}
            />
            <button type="submit" className={secondaryButtonClass}>
              Mentés
            </button>
          </form>
        </details>
      ) : null}

      {anySetup ? (
        <p className="text-sm text-zinc-600 dark:text-zinc-400">
          {!veglegesitve
            ? "🛏 Az ágyak a véglegesítés után számítanak."
            : agyNelkul.length > 0
              ? `🛏 ${agyNelkul.length} főnek nincs ágya: ${agyNelkul
                  .map(nameOf)
                  .join(", ")}`
              : "🛏 Mindenkinek van ágya. ✓"}
        </p>
      ) : (
        <p className="text-sm text-zinc-500">
          Még nincs szoba.
          {isSzervezo
            ? ""
            : " A szervező fogja felvenni a szobákat és az ágyakat."}
        </p>
      )}

      {szallas.szobak.length > 0 ? (
        <ul className="flex flex-col gap-4">
          {szallas.szobak.map((szoba) => (
            <SzobaItem
              key={szoba.id}
              esemenyId={esemeny.id}
              szoba={szoba}
              szobak={szallas.szobak}
              canSetup={canSetup}
              canClaim={canClaim}
              hasBed={szallas.sajatAgyId !== ""}
              sajatAgyId={veglegesitve ? szallas.sajatAgyId : ""}
              canMove={canMove}
            />
          ))}
        </ul>
      ) : null}

      {masholAlszik.length > 0 ? (
        <p className="text-sm">
          Máshol alszik: {masholAlszik.map(nameOf).join(", ")}
        </p>
      ) : null}

      {canClaim ? (
        <form
          action={setMasholAlszikAction.bind(
            null,
            esemeny.id,
            !szallas.sajatMasholAlszik,
          )}
        >
          <button type="submit" className={secondaryButtonClass}>
            {szallas.sajatMasholAlszik ? "Mégis itt alszom" : "Máshol alszom"}
          </button>
        </form>
      ) : null}

      {canMove && allAgyak.length > 0 ? (
        <ul className="flex flex-col gap-2 text-sm">
          {[
            ...agyNelkul.map((s) => ({ szemely: s, mashol: false })),
            ...masholAlszik.map((s) => ({ szemely: s, mashol: true })),
          ]
            .filter(({ szemely }) => szemely.felhasznaloId !== "")
            .map(({ szemely: s, mashol }) => (
              <li key={s.felhasznaloId} className="flex items-center gap-2">
                <span>
                  {nameOf(s)}
                  {mashol ? " (máshol alszik)" : ""}
                </span>
                <MoveForm
                  esemenyId={esemeny.id}
                  szemely={s}
                  szobak={szallas.szobak}
                  currentAgyId=""
                  label="Beoszt"
                />
              </li>
            ))}
        </ul>
      ) : null}

      {canSetup ? (
        <form
          action={addSzobaAction.bind(null, esemeny.id)}
          className="flex flex-wrap items-end gap-2"
        >
          <label className="flex flex-col gap-1 text-sm">
            Új szoba
            <input
              name="name"
              required
              maxLength={60}
              placeholder="pl. Emeleti szoba"
              className={inputClass}
            />
          </label>
          <button type="submit" className={secondaryButtonClass}>
            Szoba hozzáadása
          </button>
        </form>
      ) : null}

      <VendegAgyak
        esemenyId={esemeny.id}
        szallas={szallas}
        vendegek={sajatVendegResztvevok(esemeny, felhasznaloId)}
      />
    </div>
  );
}

function MoveForm({
  esemenyId,
  szemely,
  szobak,
  currentAgyId,
  label,
}: {
  esemenyId: string;
  szemely: Szemely;
  szobak: SzobaAllas[];
  currentAgyId: string;
  label: string;
}) {
  const groups = szobak
    .map((szoba) => ({
      szoba,
      agyak: szoba.agyak.filter((a) => a.id !== currentAgyId && a.szabad > 0),
    }))
    .filter((g) => g.agyak.length > 0);
  if (groups.length === 0) {
    return null;
  }
  return (
    <form
      action={moveAlvoAction.bind(null, esemenyId, szemely.felhasznaloId)}
      className="flex items-center gap-2"
    >
      <select
        name="agyId"
        aria-label={`${nameOf(szemely)}: melyik ágyba`}
        className={inputClass}
      >
        {groups.map(({ szoba, agyak }) => (
          <optgroup key={szoba.id} label={szoba.name}>
            {agyak.map((a) => (
              <option key={a.id} value={a.id}>
                {a.label === "" ? "Ágy" : a.label} ({a.szabad} szabad)
              </option>
            ))}
          </optgroup>
        ))}
      </select>
      <button type="submit" className={secondaryButtonClass}>
        {label}
      </button>
    </form>
  );
}

function SzobaItem({
  esemenyId,
  szoba,
  szobak,
  canSetup,
  canClaim,
  hasBed,
  sajatAgyId,
  canMove,
}: {
  esemenyId: string;
  szoba: SzobaAllas;
  szobak: SzobaAllas[];
  canSetup: boolean;
  canClaim: boolean;
  hasBed: boolean;
  sajatAgyId: string;
  canMove: boolean;
}) {
  const alvokCount = szoba.agyak.reduce((n, a) => n + a.alvok.length, 0);
  return (
    <li className="flex flex-col gap-2 border-t border-zinc-200 pt-3 first:border-t-0 first:pt-0 dark:border-zinc-800">
      <h4 className="font-medium">{szoba.name}</h4>

      {szoba.agyak.length > 0 ? (
        <ul className="flex flex-col gap-3">
          {szoba.agyak.map((agy) => (
            <AgyItem
              key={agy.id}
              esemenyId={esemenyId}
              agy={agy}
              szobak={szobak}
              canSetup={canSetup}
              canClaim={canClaim}
              hasBed={hasBed}
              mine={sajatAgyId === agy.id}
              canMove={canMove}
            />
          ))}
        </ul>
      ) : (
        <p className="text-sm text-zinc-500">Még nincs ágy ebben a szobában.</p>
      )}

      {canSetup ? (
        <details className="text-sm">
          <summary className="cursor-pointer text-zinc-500">
            Szoba szerkesztése
          </summary>
          <div className="mt-2 flex flex-col gap-3">
            <form
              action={addAgyAction.bind(null, esemenyId, szoba.id)}
              className="flex flex-wrap items-end gap-2"
            >
              <select
                name="capacity"
                defaultValue="1"
                aria-label="Új ágy férőhelye"
                className={inputClass}
              >
                <option value="1">Egyszemélyes</option>
                <option value="2">Kétszemélyes</option>
              </select>
              <input
                name="label"
                maxLength={40}
                placeholder="Megjegyzés (pl. emeletes, alsó)"
                aria-label="Új ágy megjegyzése"
                className={inputClass}
              />
              <button type="submit" className={secondaryButtonClass}>
                Ágy hozzáadása
              </button>
            </form>
            <form
              action={renameSzobaAction.bind(null, esemenyId, szoba.id)}
              className="flex flex-wrap items-end gap-2"
            >
              <input
                name="name"
                required
                maxLength={60}
                defaultValue={szoba.name}
                aria-label="A szoba neve"
                className={inputClass}
              />
              <button type="submit" className={secondaryButtonClass}>
                Átnevezés
              </button>
            </form>
            <ConfirmDelete
              action={deleteSzobaAction.bind(null, esemenyId, szoba.id)}
              label="Szoba törlése"
              warning={
                alvokCount > 0
                  ? `A szobával együtt az ágyai törlődnek, és a ${alvokCount} bent alvó értesítést kap.`
                  : "A szobával együtt az ágyai is törlődnek."
              }
            />
          </div>
        </details>
      ) : null}
    </li>
  );
}

function AgyItem({
  esemenyId,
  agy,
  szobak,
  canSetup,
  canClaim,
  hasBed,
  mine,
  canMove,
}: {
  esemenyId: string;
  agy: AgyAllas;
  szobak: SzobaAllas[];
  canSetup: boolean;
  canClaim: boolean;
  hasBed: boolean;
  mine: boolean;
  canMove: boolean;
}) {
  const label = agy.label === "" ? "Ágy" : agy.label;
  return (
    <li className="flex flex-col gap-1.5">
      <div className="flex items-start justify-between gap-4">
        <span>
          {label}{" "}
          <span className="text-sm text-zinc-500">
            ({agy.capacity === 2 ? "kétszemélyes" : "egyszemélyes"})
          </span>
        </span>
        <span
          className="shrink-0 text-sm text-zinc-600 dark:text-zinc-400"
          role="img"
          aria-label={`${agy.alvok.length}/${agy.capacity} hely foglalt`}
        >
          {"●".repeat(agy.alvok.length)}
          {"○".repeat(agy.szabad)}
        </span>
      </div>

      {agy.alvok.length > 0 ? (
        <ul className="flex flex-col gap-1 text-sm">
          {agy.alvok.map((u) => (
            <li
              key={szemelyKey(u)}
              className="flex flex-wrap items-center gap-2"
            >
              <span>{nameOf(u)}</span>
              {canMove && u.felhasznaloId !== "" ? (
                <>
                  <MoveForm
                    esemenyId={esemenyId}
                    szemely={u}
                    szobak={szobak}
                    currentAgyId={agy.id}
                    label="Áthelyez"
                  />
                  <form
                    action={releaseFoglalasAction.bind(
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

      {canClaim && !hasBed && agy.szabad > 0 ? (
        <form action={claimAgyAction.bind(null, esemenyId, agy.id)}>
          <button type="submit" className={secondaryButtonClass}>
            Ide alszom
          </button>
        </form>
      ) : null}
      {mine ? (
        <form action={releaseFoglalasAction.bind(null, esemenyId, "")}>
          <button type="submit" className={secondaryButtonClass}>
            Kiköltözöm
          </button>
        </form>
      ) : null}

      {canSetup ? (
        <details className="text-sm">
          <summary className="cursor-pointer text-zinc-500">
            Ágy szerkesztése
          </summary>
          <div className="mt-2 flex flex-col gap-2">
            <form
              action={updateAgyAction.bind(null, esemenyId, agy.id)}
              className="flex flex-wrap items-end gap-2"
            >
              <select
                name="capacity"
                defaultValue={String(agy.capacity)}
                aria-label="Férőhely"
                className={inputClass}
              >
                <option value="1" disabled={agy.alvok.length > 1}>
                  Egyszemélyes
                </option>
                <option value="2">Kétszemélyes</option>
              </select>
              <input
                name="label"
                maxLength={40}
                defaultValue={agy.label}
                aria-label="Megjegyzés"
                className={inputClass}
              />
              <button type="submit" className={secondaryButtonClass}>
                Mentés
              </button>
            </form>
            <ConfirmDelete
              action={deleteAgyAction.bind(null, esemenyId, agy.id)}
              label="Ágy törlése"
              warning={
                agy.alvok.length > 0
                  ? "Az ágy törlésével a bent alvók ágy nélkül maradnak."
                  : "Biztosan törlöd az ágyat?"
              }
            />
          </div>
        </details>
      ) : null}
    </li>
  );
}
