import Link from "next/link";
import { allapotLabel, formatIdopont } from "@/app/esemenyek/format";
import { getEsemenyek } from "@/lib/esemeny";
import { buttonClass, cardClass } from "@/lib/ui";

/** The Csoport's Események, newest first, and the way to propose one. */
export async function EsemenySection({ csoportId }: { csoportId: string }) {
  const esemenyek = (await getEsemenyek(csoportId)) ?? [];

  return (
    <section className="flex flex-col gap-3">
      <div className="flex items-center justify-between gap-4">
        <h2 className="text-xl font-semibold">Események</h2>
        <Link
          href={`/csoportok/${csoportId}/uj-esemeny`}
          className={`${buttonClass} py-2 text-sm`}
        >
          Új esemény
        </Link>
      </div>
      {esemenyek.length === 0 ? (
        <p className="text-zinc-500">Még nincs esemény.</p>
      ) : (
        <ul className="flex flex-col gap-2">
          {esemenyek.map((esemeny) => (
            <li key={esemeny.id}>
              <Link
                href={`/esemenyek/${esemeny.id}`}
                className={`${cardClass} flex flex-col gap-1 transition-colors hover:bg-zinc-50 dark:hover:bg-zinc-900`}
              >
                <span className="font-medium">{esemeny.title}</span>
                <span className="text-sm text-zinc-500">
                  {allapotLabel[esemeny.allapot]} · {esemeny.idopontCount}{" "}
                  időpont
                  {esemeny.firstStart
                    ? ` · első: ${formatIdopont(esemeny.firstStart, null)}`
                    : ""}
                </span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
