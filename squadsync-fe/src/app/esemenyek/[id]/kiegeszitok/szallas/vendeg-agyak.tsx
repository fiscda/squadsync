import type { Resztvevo } from "@/lib/backend";
import type { Szallas } from "@/lib/kiegeszito/szallas";
import { secondaryButtonClass } from "@/lib/ui";
import {
  claimAgyForVendegAction,
  releaseFoglalasForVendegAction,
  setMasholAlszikForVendegAction,
} from "./actions";

const inputClass =
  "rounded-lg border border-zinc-300 bg-transparent px-3 py-1.5 text-sm dark:border-zinc-700";

/** The viewer gives their name-only Vendégek a bed, or marks them elsewhere. */
export function VendegAgyak({
  esemenyId,
  szallas,
  vendegek,
}: {
  esemenyId: string;
  szallas: Szallas;
  vendegek: Resztvevo[];
}) {
  if (vendegek.length === 0) {
    return null;
  }
  const agyak = szallas.szobak.flatMap((sz) =>
    sz.agyak.map((agy) => ({ agy, szoba: sz.name })),
  );
  const szabad = agyak.filter(({ agy }) => agy.szabad > 0);
  return (
    <div className="flex flex-col gap-2 text-sm">
      <h4 className="font-medium">A vendégeid</h4>
      <ul className="flex flex-col gap-2">
        {vendegek.map((v) => {
          const hol = agyak.find(({ agy }) =>
            agy.alvok.some((a) => a.vendegId === v.vendegId),
          );
          const mashol = szallas.masholAlszik.some(
            (m) => m.vendegId === v.vendegId,
          );
          return (
            <li key={v.vendegId} className="flex flex-wrap items-center gap-2">
              <span>
                {v.name}:{" "}
                {hol ? hol.szoba : mashol ? "máshol alszik" : "nincs ágya"}
              </span>
              {hol ? (
                <form
                  action={releaseFoglalasForVendegAction.bind(
                    null,
                    esemenyId,
                    v.vendegId,
                  )}
                >
                  <button type="submit" className={secondaryButtonClass}>
                    Kiköltözik
                  </button>
                </form>
              ) : szabad.length > 0 ? (
                <form
                  action={claimAgyForVendegAction.bind(
                    null,
                    esemenyId,
                    v.vendegId,
                  )}
                  className="flex items-center gap-2"
                >
                  <select
                    name="agyId"
                    aria-label={`Ágy ${v.name} számára`}
                    className={inputClass}
                  >
                    {szabad.map(({ agy, szoba }) => (
                      <option key={agy.id} value={agy.id}>
                        {`${szoba}${agy.label !== "" ? `, ${agy.label}` : ""} (${agy.szabad} hely)`}
                      </option>
                    ))}
                  </select>
                  <button type="submit" className={secondaryButtonClass}>
                    Ide alszik
                  </button>
                </form>
              ) : null}
              <form
                action={setMasholAlszikForVendegAction.bind(
                  null,
                  esemenyId,
                  v.vendegId,
                  !mashol,
                )}
              >
                <button type="submit" className={secondaryButtonClass}>
                  {mashol ? "Mégsem alszik máshol" : "Máshol alszik"}
                </button>
              </form>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
