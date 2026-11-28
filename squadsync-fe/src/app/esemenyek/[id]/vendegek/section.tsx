import { CopyLink } from "@/app/csoportok/[id]/copy-link";
import type { EsemenyDetail, Valasz, Vendeg } from "@/lib/backend";
import {
  buttonClass,
  cardClass,
  formatDate,
  secondaryButtonClass,
} from "@/lib/ui";
import { getEsemenyMeghivok } from "@/lib/vendeg";
import { formatEgeszNapos, formatIdopont, valaszLabel } from "../../format";
import {
  addVendegAction,
  createEsemenyMeghivoAction,
  removeVendegAction,
  revokeEsemenyMeghivoAction,
  setVendegVisszajelzesAction,
} from "./actions";

const inputClass =
  "rounded-lg border border-zinc-300 bg-transparent px-3 py-1.5 text-sm dark:border-zinc-700";

/** Why a Vendég change was refused (see runVendegChange), or null. */
const vendegHibak: Record<string, string> = {
  vendeg_nincs:
    "Ez a vendég vagy meghívó már nem létezik. Az oldal a mostani állapotot mutatja.",
  vendeg_ervenytelen: "A vendég neve 1–60 karakter legyen.",
  vendeg_closed:
    "Vendéget csak egyeztetés alatt vagy véglegesített eseményhez lehet hívni.",
  vendeg_has_kiadas:
    "Ez a vendég szerepel egy kiadásban (fizetőként vagy a megosztásban). Előbb módosítsd vagy töröld azt a kiadást.",
  vendeg_nincs_jog:
    "Ezt csak az teheti meg, aki a vendéget hozzáadta, vagy az esemény szervezője.",
  vendeg_allapot:
    "Ez most nem lehetséges, mert az esemény állapota közben megváltozott.",
};

/** The notice for a ?hiba= of a Vendég change, or null for another one. */
export function vendegHibaText(
  hiba: string | string[] | undefined,
): string | null {
  return typeof hiba === "string" && Object.hasOwn(vendegHibak, hiba)
    ? vendegHibak[hiba]
    : null;
}

const valaszok: Valasz[] = ["igen", "talan", "nem"];

/**
 * The Vendégek of an Esemény as a Tag sees them: Esemény-meghívó links to
 * copy and revoke, the Vendégek with removal, a form to add a name-only
 * Vendég, and the answers the viewer gives for their own name-only ones.
 */
export async function VendegekSection({
  esemeny,
  felhasznaloId,
}: {
  esemeny: EsemenyDetail;
  felhasznaloId: string;
}) {
  const joinable =
    esemeny.allapot === "egyeztetes" || esemeny.allapot === "veglegesitve";
  const meghivok = joinable
    ? ((await getEsemenyMeghivok(esemeny.id)) ?? [])
    : [];
  const mine = esemeny.vendegek.filter((v) => v.canAnswer);
  if (!joinable && esemeny.vendegek.length === 0) {
    return null;
  }
  // The public address of the web app, so the copied link works anywhere.
  const publicUrl = process.env.BETTER_AUTH_URL;
  if (!publicUrl) {
    throw new Error("BETTER_AUTH_URL is not set (see .env.example)");
  }
  const baseUrl = publicUrl.replace(/\/$/, "");

  return (
    <section className="flex flex-col gap-4">
      <h2 className="text-xl font-semibold">
        Vendégek ({esemeny.vendegek.length})
      </h2>
      <p className="text-sm text-zinc-600 dark:text-zinc-400">
        A vendégek csak ezt az eseményt látják, a csoportot és a többi eseményt
        nem. A helyezésbe nem számítanak bele, a létszámba igen.
      </p>

      {esemeny.vendegek.length > 0 ? (
        <ul className="flex flex-col gap-2">
          {esemeny.vendegek.map((v) => (
            <VendegItem
              key={v.id}
              esemenyId={esemeny.id}
              vendeg={v}
              felhasznaloId={felhasznaloId}
            />
          ))}
        </ul>
      ) : null}

      {mine.length > 0 ? (
        <VendegValaszok esemeny={esemeny} vendegek={mine} />
      ) : null}

      {joinable ? (
        <form
          action={addVendegAction.bind(null, esemeny.id)}
          className="flex flex-wrap items-end gap-2"
        >
          <label className="flex flex-col gap-1 text-sm">
            Vendég név szerint (akinek nincs Discordja)
            <input
              name="name"
              required
              maxLength={60}
              placeholder="pl. Nagyi"
              className={inputClass}
            />
          </label>
          <button type="submit" className={secondaryButtonClass}>
            Hozzáadás
          </button>
        </form>
      ) : null}

      {joinable ? (
        <div className="flex flex-col gap-3">
          <h3 className="font-semibold">Esemény-meghívó linkek</h3>
          <p className="text-sm text-zinc-600 dark:text-zinc-400">
            Aki megnyitja a linket és belép Discorddal, vendég lesz ezen az
            eseményen. A link lezajlás vagy elmaradás után nem működik.
          </p>
          {meghivok.length === 0 ? (
            <p className="text-sm text-zinc-500">
              Még nincs érvényes esemény-meghívó link.
            </p>
          ) : (
            <ul className="flex flex-col gap-3">
              {meghivok.map((m) => (
                <li key={m.id} className={`${cardClass} flex flex-col gap-3`}>
                  <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
                    <CopyLink
                      url={`${baseUrl}/esemeny-meghivo/${encodeURIComponent(m.token)}`}
                    />
                    <form
                      action={revokeEsemenyMeghivoAction.bind(
                        null,
                        esemeny.id,
                        m.id,
                      )}
                    >
                      <button type="submit" className={secondaryButtonClass}>
                        Visszavonás
                      </button>
                    </form>
                  </div>
                  <p className="text-xs text-zinc-500">
                    Létrehozva: {formatDate(m.createdAt)}
                  </p>
                </li>
              ))}
            </ul>
          )}
          <form action={createEsemenyMeghivoAction.bind(null, esemeny.id)}>
            <button type="submit" className={buttonClass}>
              Új esemény-meghívó link
            </button>
          </form>
        </div>
      ) : null}
    </section>
  );
}

function VendegItem({
  esemenyId,
  vendeg,
  felhasznaloId,
}: {
  esemenyId: string;
  vendeg: Vendeg;
  felhasznaloId: string;
}) {
  const who =
    vendeg.kind === "felhasznalo"
      ? "meghívó linkkel csatlakozott"
      : vendeg.addedBy === felhasznaloId
        ? "a te vendéged"
        : `${vendeg.addedByName} vendége`;
  return (
    <li className="flex flex-wrap items-center gap-2 text-sm">
      <span className="font-medium">{vendeg.name}</span>
      <span className="text-zinc-500">({who})</span>
      {vendeg.canRemove ? (
        <form action={removeVendegAction.bind(null, esemenyId, vendeg.id)}>
          <button
            type="submit"
            className={`${secondaryButtonClass} text-red-700 dark:text-red-400`}
          >
            Eltávolítás
          </button>
        </form>
      ) : null}
    </li>
  );
}

const selectedClass: Record<Valasz, string> = {
  igen: "border-green-600 bg-green-600 text-white",
  talan: "border-amber-500 bg-amber-500 text-white",
  nem: "border-red-600 bg-red-600 text-white",
};

/** The viewer answers the answerable Időpontok for their own Vendégek. */
function VendegValaszok({
  esemeny,
  vendegek,
}: {
  esemeny: EsemenyDetail;
  vendegek: Vendeg[];
}) {
  const answerable = esemeny.idopontok.filter((i) => i.answerable);
  if (answerable.length === 0) {
    return null;
  }
  return (
    <div className="flex flex-col gap-3">
      <h3 className="font-semibold">Válaszok a vendégeid nevében</h3>
      {vendegek.map((v) => (
        <div key={v.id} className={`${cardClass} flex flex-col gap-2`}>
          <span className="font-medium">{v.name}</span>
          <ul className="flex flex-col gap-2">
            {answerable.map((i) => {
              const valasz =
                i.visszajelzesek.find((vj) => vj.vendegId === v.id)?.valasz ??
                null;
              return (
                <li key={i.id} className="flex flex-wrap items-center gap-2">
                  <span className="text-sm">
                    {i.egeszNapos && i.end !== null
                      ? formatEgeszNapos(i.start, i.end)
                      : formatIdopont(i.start, i.end)}
                  </span>
                  {valaszok.map((valaszOpt) => (
                    <form
                      key={valaszOpt}
                      action={setVendegVisszajelzesAction.bind(
                        null,
                        esemeny.id,
                        i.id,
                        v.id,
                        valaszOpt,
                      )}
                    >
                      <button
                        type="submit"
                        aria-pressed={valasz === valaszOpt}
                        aria-label={`${v.name}: ${valaszLabel[valaszOpt]}`}
                        className={`rounded-full border px-3 py-1 text-sm font-medium transition-opacity hover:opacity-80 ${
                          valasz === valaszOpt
                            ? selectedClass[valaszOpt]
                            : "border-zinc-300 dark:border-zinc-700"
                        }`}
                      >
                        {valaszLabel[valaszOpt]}
                      </button>
                    </form>
                  ))}
                </li>
              );
            })}
          </ul>
        </div>
      ))}
    </div>
  );
}
