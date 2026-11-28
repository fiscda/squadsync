import type { Resztvevo } from "@/lib/backend";
import type { Autok } from "@/lib/kiegeszito/autok";
import { secondaryButtonClass } from "@/lib/ui";
import {
  claimUlesForVendegAction,
  releaseUlesForVendegAction,
  setSajatUtonForVendegAction,
} from "./actions";

const inputClass =
  "rounded-lg border border-zinc-300 bg-transparent px-3 py-1.5 text-sm dark:border-zinc-700";

/** The viewer seats their name-only Vendégek, or marks them saját úton. */
export function VendegUlesek({
  esemenyId,
  autok,
  vendegek,
}: {
  esemenyId: string;
  autok: Autok;
  vendegek: Resztvevo[];
}) {
  if (vendegek.length === 0) {
    return null;
  }
  const szabad = autok.autok.filter((a) => a.szabad > 0);
  return (
    <div className="flex flex-col gap-2 text-sm">
      <h4 className="font-medium">A vendégeid</h4>
      <ul className="flex flex-col gap-2">
        {vendegek.map((v) => {
          const auto = autok.autok.find((a) =>
            a.utasok.some((u) => u.vendegId === v.vendegId),
          );
          const uton = autok.sajatUton.some((s) => s.vendegId === v.vendegId);
          return (
            <li key={v.vendegId} className="flex flex-wrap items-center gap-2">
              <span>
                {v.name}:{" "}
                {auto
                  ? `${auto.soforName} autójában`
                  : uton
                    ? "saját úton"
                    : "nincs helye"}
              </span>
              {auto ? (
                <form
                  action={releaseUlesForVendegAction.bind(
                    null,
                    esemenyId,
                    v.vendegId,
                  )}
                >
                  <button type="submit" className={secondaryButtonClass}>
                    Kiszáll
                  </button>
                </form>
              ) : szabad.length > 0 ? (
                <form
                  action={claimUlesForVendegAction.bind(
                    null,
                    esemenyId,
                    v.vendegId,
                  )}
                  className="flex items-center gap-2"
                >
                  <select
                    name="autoId"
                    aria-label={`Autó ${v.name} számára`}
                    className={inputClass}
                  >
                    {szabad.map((a) => (
                      <option key={a.id} value={a.id}>
                        {a.soforName} autója ({a.szabad} hely)
                      </option>
                    ))}
                  </select>
                  <button type="submit" className={secondaryButtonClass}>
                    Beültetem
                  </button>
                </form>
              ) : null}
              <form
                action={setSajatUtonForVendegAction.bind(
                  null,
                  esemenyId,
                  v.vendegId,
                  !uton,
                )}
              >
                <button type="submit" className={secondaryButtonClass}>
                  {uton ? "Mégsem saját úton" : "Saját úton megy"}
                </button>
              </form>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
