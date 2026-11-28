import Link from "next/link";
import { notFound } from "next/navigation";
import { Suspense } from "react";
import {
  type EsemenyDetail,
  type Idopont,
  resztvevoKey,
  type Valasz,
} from "@/lib/backend";
import { getSzervezoEsemenyek } from "@/lib/csoport";
import { getEsemeny } from "@/lib/esemeny";
import { requireFelhasznalo } from "@/lib/felhasznalo";
import { cardClass, linkClass, secondaryButtonClass } from "@/lib/ui";
import {
  type AllapotHiba,
  confirmIdopontAction,
  reopenEgyeztetesAction,
  setVisszajelzesAction,
} from "../actions";
import {
  allapotLabel,
  formatEgeszNapos,
  formatIdopont,
  formatMoment,
  toLocalInputs,
  valaszLabel,
} from "../format";
import { MAX_IDOPONTOK } from "../limits";
import { AddIdopont } from "./add-idopont";
import { CancelEsemeny } from "./cancel-esemeny";
import { CopyEsemeny } from "./copy-esemeny";
import { EditEsemeny } from "./edit-esemeny";
import { HandOverSzervezo } from "./hand-over-szervezo";
import { HataridoForm } from "./hatarido-form";
import { kiegeszitoHibaText } from "./kiegeszitok/registry";
import { KiegeszitokSection } from "./kiegeszitok/section";
import { IdopontRaeresLine } from "./raeres-line";
import { RemoveIdopont } from "./remove-idopont";
import { VendegekSection, vendegHibaText } from "./vendegek/section";

export default function EsemenyPage({
  params,
  searchParams,
}: PageProps<"/esemenyek/[id]">) {
  return (
    <main className="mx-auto flex w-full max-w-xl flex-1 flex-col gap-8 px-4 py-12">
      <Suspense fallback={<p className="text-zinc-500">Betöltés…</p>}>
        {Promise.all([params, searchParams]).then(([{ id }, { hiba }]) => (
          <Esemeny esemenyId={id} hiba={hiba} />
        ))}
      </Suspense>
    </main>
  );
}

/** Why the Szervező's last change was refused (see changeAllapot). */
const hibaText: Record<AllapotHiba, string> = {
  szervezo: "Ezt csak az esemény szervezője teheti meg.",
  allapot:
    "Ez most nem lehetséges, mert az esemény állapota közben megváltozott. Az oldal már a mostani állapotot mutatja.",
  nincs_jog:
    "Ezt az időpontot csak az törölheti, aki hozzáadta, vagy az esemény szervezője.",
  not_egyeztetes: "Időpontot csak egyeztetés alatt lehet törölni.",
  idopont_has_answers:
    "Erre az időpontra már válaszoltak, ezért csak az esemény szervezője törölheti.",
  last_idopont: "Az utolsó időpontot nem lehet törölni.",
  nem_jelolheto:
    "A szervezést csak olyan tagnak lehet átadni, aki válaszolt az eseményre. Az átadás nem történt meg.",
  masolas: "Az esemény másolása most nem sikerült. Próbáld újra később.",
};

function isAllapotHiba(value: unknown): value is AllapotHiba {
  return typeof value === "string" && Object.hasOwn(hibaText, value);
}

// Reads the session and the route params, so it streams in behind Suspense.
async function Esemeny({
  esemenyId,
  hiba,
}: {
  esemenyId: string;
  hiba: string | string[] | undefined;
}) {
  const esemeny = await getEsemeny(esemenyId);
  if (!esemeny) {
    // Not a Tag, or no such Esemény: the same answer either way.
    notFound();
  }
  const felhasznalo = await requireFelhasznalo();
  const { helyszin } = esemeny;
  const isSzervezo = esemeny.szervezoId === felhasznalo.id;
  const hasResztvevok =
    esemeny.allapot === "veglegesitve" || esemeny.allapot === "lezajlott";
  // Who the Szervező could hand the role to (Tags with a Visszajelzés on it).
  const utodok = isSzervezo
    ? ((await getSzervezoEsemenyek(esemeny.csoportId, felhasznalo.id)).find(
        (e) => e.id === esemeny.id,
      )?.utodok ?? [])
    : [];
  // A Vendég sees this Esemény only: nothing of its Csoport, and nothing
  // that only Tags may do.
  const isVendeg = esemeny.viewerIsVendeg;
  const notice = isAllapotHiba(hiba)
    ? hibaText[hiba]
    : (vendegHibaText(hiba) ?? kiegeszitoHibaText(hiba));

  return (
    <>
      <div className="flex flex-col gap-2">
        {isVendeg ? (
          <p className="text-sm text-zinc-500">
            Vendégként vagy meghívva erre az eseményre.
          </p>
        ) : (
          <Link
            href={`/csoportok/${esemeny.csoportId}`}
            className={`${linkClass} text-sm`}
          >
            ← {esemeny.csoportName}
          </Link>
        )}
        <h1 className="text-3xl font-semibold tracking-tight">
          {esemeny.title}
        </h1>
        <p className="text-sm text-zinc-500">
          {allapotLabel[esemeny.allapot]} · Szervező: {esemeny.szervezoName}
        </p>
        {isSzervezo && esemeny.allapot !== "elmaradt" ? (
          <EditEsemeny
            esemenyId={esemeny.id}
            title={esemeny.title}
            description={esemeny.description}
            helyszinName={helyszin.name}
            helyszinUrl={helyszin.url}
          />
        ) : null}
        {esemeny.hatarido !== null && esemeny.allapot === "egyeztetes" ? (
          <p className="text-sm text-zinc-500">
            Szavazási határidő: {formatMoment(esemeny.hatarido)}
          </p>
        ) : null}
      </div>

      {notice !== null ? (
        <p
          role="alert"
          className="rounded-2xl border border-amber-300 bg-amber-50 p-4 text-sm text-amber-900 dark:border-amber-800 dark:bg-amber-950 dark:text-amber-200"
        >
          {notice}
        </p>
      ) : null}

      {esemeny.allapot === "elmaradt" ? (
        <p className="rounded-2xl border border-red-300 bg-red-50 p-4 font-medium text-red-800 dark:border-red-900 dark:bg-red-950 dark:text-red-300">
          ❌ Ez az esemény elmaradt. Már nem lehet rá válaszolni.
        </p>
      ) : null}

      {isSzervezo ? (
        <SzervezoControls esemeny={esemeny} utodok={utodok} />
      ) : null}

      {esemeny.description !== "" ? (
        <p className="whitespace-pre-line">{esemeny.description}</p>
      ) : null}

      {helyszin.name !== "" || helyszin.url !== "" ? (
        <section className="flex flex-col gap-1">
          <h2 className="text-xl font-semibold">Helyszín</h2>
          {helyszin.name !== "" ? <p>{helyszin.name}</p> : null}
          {helyszin.url !== "" ? (
            <a
              href={helyszin.url}
              target="_blank"
              rel="noopener noreferrer"
              className={`${linkClass} self-start text-sm`}
            >
              Megnyitás térképen ↗
            </a>
          ) : null}
        </section>
      ) : null}

      {hasResztvevok ? <Resztvevok esemeny={esemeny} /> : null}

      <section className="flex flex-col gap-3">
        <h2 className="text-xl font-semibold">Időpontok</h2>
        <p className="text-sm text-zinc-600 dark:text-zinc-400">
          Budapesti idő szerint. A helyezést a tagok igen, majd talán, majd
          kevesebb nem válaszai döntik el; a vendégeké nem számít bele.
        </p>
        <ul className="flex flex-col gap-3">
          {esemeny.idopontok.map((idopont) => (
            <IdopontCard
              key={idopont.id}
              esemenyId={esemeny.id}
              idopont={idopont}
              canConfirm={isSzervezo && esemeny.allapot === "egyeztetes"}
              dimmed={hasResztvevok && !idopont.confirmed}
              showRaeres={!isVendeg}
            />
          ))}
        </ul>
        {!isVendeg &&
        esemeny.allapot === "egyeztetes" &&
        esemeny.idopontok.length < MAX_IDOPONTOK ? (
          <AddIdopont esemenyId={esemeny.id} />
        ) : null}
      </section>

      {isVendeg ? null : (
        <VendegekSection esemeny={esemeny} felhasznaloId={felhasznalo.id} />
      )}

      {isVendeg ? null : (
        <section className="flex flex-col gap-2">
          <CopyEsemeny esemenyId={esemeny.id} />
          <p className="text-sm text-zinc-500">
            Új esemény jön létre ugyanezzel a címmel, helyszínnel és bekapcsolt
            kiegészítőkkel, az időpontok egy héttel későbbre téve. Te leszel a
            szervezője.
          </p>
        </section>
      )}

      <KiegeszitokSection
        esemeny={esemeny}
        felhasznaloId={felhasznalo.id}
        isSzervezo={isSzervezo}
      />
    </>
  );
}

/**
 * Reopening, cancelling and handing the role over; confirming is on each
 * Időpont.
 */
function SzervezoControls({
  esemeny,
  utodok,
}: {
  esemeny: EsemenyDetail;
  utodok: { felhasznaloId: string; name: string }[];
}) {
  const canReopen = esemeny.allapot === "veglegesitve";
  const canCancel =
    esemeny.allapot === "egyeztetes" || esemeny.allapot === "veglegesitve";
  if (!canReopen && !canCancel) {
    return null;
  }
  return (
    <section className={`${cardClass} flex flex-col gap-3`}>
      <h2 className="font-semibold">Szervezőként</h2>
      {esemeny.allapot === "egyeztetes" ? (
        <p className="text-sm text-zinc-600 dark:text-zinc-400">
          Válaszd ki a végleges időpontot a „Véglegesítés” gombbal. Bármelyiket
          választhatod, nem csak az elsőt.
        </p>
      ) : null}
      {esemeny.allapot === "egyeztetes" ? (
        <HataridoForm
          key={esemeny.hatarido ?? ""}
          esemenyId={esemeny.id}
          {...(esemeny.hatarido === null
            ? { date: "", time: "" }
            : toLocalInputs(esemeny.hatarido))}
        />
      ) : null}
      <div className="flex flex-wrap items-start gap-2">
        {canReopen ? (
          <form action={reopenEgyeztetesAction.bind(null, esemeny.id)}>
            <button type="submit" className={secondaryButtonClass}>
              Egyeztetés újranyitása
            </button>
          </form>
        ) : null}
        {canCancel ? <CancelEsemeny esemenyId={esemeny.id} /> : null}
      </div>
      {canCancel ? (
        <HandOverSzervezo esemenyId={esemeny.id} utodok={utodok} />
      ) : null}
    </section>
  );
}

function Resztvevok({ esemeny }: { esemeny: EsemenyDetail }) {
  return (
    <section className="flex flex-col gap-2">
      <h2 className="text-xl font-semibold">
        Résztvevők ({esemeny.resztvevok.length})
      </h2>
      {esemeny.resztvevok.length === 0 ? (
        <p className="text-sm text-zinc-500">
          Még senki nem mondott igent a végleges időpontra.
        </p>
      ) : (
        <ul className="flex flex-wrap gap-2">
          {esemeny.resztvevok.map((r) => (
            <li
              key={resztvevoKey(r)}
              className="rounded-full bg-green-100 px-3 py-1 text-sm text-green-900 dark:bg-green-950 dark:text-green-200"
            >
              {r.name}
              {r.isVendeg ? (
                <span className="text-green-700 dark:text-green-400">
                  {" "}
                  (vendég)
                </span>
              ) : null}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

const valaszok: Valasz[] = ["igen", "talan", "nem"];

/** A Tags' count with the Vendégek' on top, e.g. "3 (+1 vendég)". */
function withVendegek(tagok: number, vendegek: number): string {
  return vendegek > 0 ? `${tagok} (+${vendegek} vendég)` : String(tagok);
}

const selectedClass: Record<Valasz, string> = {
  igen: "border-green-600 bg-green-600 text-white",
  talan: "border-amber-500 bg-amber-500 text-white",
  nem: "border-red-600 bg-red-600 text-white",
};

function IdopontCard({
  esemenyId,
  idopont,
  canConfirm,
  dimmed,
  showRaeres,
}: {
  esemenyId: string;
  idopont: Idopont;
  canConfirm: boolean;
  dimmed: boolean;
  /** Ráérés is for Tags only: a Vendég sees nobody's. */
  showRaeres: boolean;
}) {
  const highlight = idopont.confirmed
    ? "border-2 border-green-600 dark:border-green-500"
    : dimmed
      ? "opacity-60"
      : "";
  return (
    <li className={`${cardClass} ${highlight} flex flex-col gap-3`}>
      {idopont.confirmed ? (
        <span className="self-start rounded-full bg-green-600 px-3 py-0.5 text-xs font-semibold text-white">
          ✅ Végleges időpont
        </span>
      ) : null}
      <div className="flex items-start justify-between gap-4">
        <span className="font-medium">
          {idopont.egeszNapos && idopont.end !== null
            ? `${formatEgeszNapos(idopont.start, idopont.end)} (egész napos)`
            : formatIdopont(idopont.start, idopont.end)}
        </span>
        <span className="shrink-0 text-lg font-semibold" title="Helyezés">
          <span className="sr-only">Helyezés: </span>
          {idopont.helyezes === null ? "–" : `${idopont.helyezes}.`}
        </span>
      </div>
      {showRaeres ? (
        <IdopontRaeresLine
          esemenyId={esemenyId}
          idopontId={idopont.id}
          answerable={idopont.answerable}
        />
      ) : null}
      <p className="text-sm text-zinc-600 dark:text-zinc-400">
        Igen: {withVendegek(idopont.igen, idopont.vendegIgen)} · Talán:{" "}
        {withVendegek(idopont.talan, idopont.vendegTalan)} · Nem:{" "}
        {withVendegek(idopont.nem, idopont.vendegNem)}
      </p>
      {idopont.visszajelzesek.length > 0 ? (
        <ul className="flex flex-col gap-1 text-sm">
          {valaszok.map((valasz) => {
            const names = idopont.visszajelzesek
              .filter((v) => v.valasz === valasz)
              .map((v) =>
                v.isTag
                  ? v.name
                  : v.isVendeg
                    ? `${v.name} (vendég)`
                    : `${v.name} (nem tag)`,
              );
            return names.length > 0 ? (
              <li key={valasz}>
                <span className="text-zinc-500">{valaszLabel[valasz]}:</span>{" "}
                {names.join(", ")}
              </li>
            ) : null;
          })}
        </ul>
      ) : null}
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-sm text-zinc-500">
          {idopont.sajat === null
            ? "Még nem válaszoltál."
            : `A válaszod: ${valaszLabel[idopont.sajat]}`}
        </span>
        {idopont.answerable
          ? valaszok.map((valasz) => (
              <form
                key={valasz}
                action={setVisszajelzesAction.bind(
                  null,
                  esemenyId,
                  idopont.id,
                  valasz,
                )}
              >
                <button
                  type="submit"
                  aria-pressed={idopont.sajat === valasz}
                  className={`rounded-full border px-4 py-1.5 text-sm font-medium transition-opacity hover:opacity-80 ${
                    idopont.sajat === valasz
                      ? selectedClass[valasz]
                      : "border-zinc-300 dark:border-zinc-700"
                  }`}
                >
                  {valaszLabel[valasz]}
                </button>
              </form>
            ))
          : null}
      </div>
      {canConfirm || idopont.removable ? (
        <div className="flex flex-wrap items-start gap-2">
          {canConfirm ? (
            <form
              action={confirmIdopontAction.bind(null, esemenyId, idopont.id)}
            >
              <button type="submit" className={secondaryButtonClass}>
                Véglegesítés
              </button>
            </form>
          ) : null}
          {idopont.removable ? (
            <RemoveIdopont
              esemenyId={esemenyId}
              idopontId={idopont.id}
              hasAnswers={idopont.visszajelzesek.length > 0}
            />
          ) : null}
        </div>
      ) : null}
    </li>
  );
}
