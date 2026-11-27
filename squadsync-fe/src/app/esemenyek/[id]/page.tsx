import Link from "next/link";
import { notFound } from "next/navigation";
import { Suspense } from "react";
import type { Idopont, Valasz } from "@/lib/backend";
import { getEsemeny } from "@/lib/esemeny";
import { cardClass, linkClass } from "@/lib/ui";
import { setVisszajelzesAction } from "../actions";
import { allapotLabel, formatIdopont, valaszLabel } from "../format";

export default function EsemenyPage({ params }: PageProps<"/esemenyek/[id]">) {
  return (
    <main className="mx-auto flex w-full max-w-xl flex-1 flex-col gap-8 px-4 py-12">
      <Suspense fallback={<p className="text-zinc-500">Betöltés…</p>}>
        {params.then(({ id }) => (
          <Esemeny esemenyId={id} />
        ))}
      </Suspense>
    </main>
  );
}

// Reads the session and the route params, so it streams in behind Suspense.
async function Esemeny({ esemenyId }: { esemenyId: string }) {
  const esemeny = await getEsemeny(esemenyId);
  if (!esemeny) {
    // Not a Tag, or no such Esemény: the same answer either way.
    notFound();
  }
  const { helyszin } = esemeny;
  const answerable = esemeny.allapot === "egyeztetes";

  return (
    <>
      <div className="flex flex-col gap-2">
        <Link
          href={`/csoportok/${esemeny.csoportId}`}
          className={`${linkClass} text-sm`}
        >
          ← {esemeny.csoportName}
        </Link>
        <h1 className="text-3xl font-semibold tracking-tight">
          {esemeny.title}
        </h1>
        <p className="text-sm text-zinc-500">
          {allapotLabel[esemeny.allapot]} · Szervező: {esemeny.szervezoName}
        </p>
      </div>

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

      <section className="flex flex-col gap-3">
        <h2 className="text-xl font-semibold">Időpontok</h2>
        <p className="text-sm text-zinc-600 dark:text-zinc-400">
          Budapesti idő szerint. A helyezést a tagok igen, majd talán, majd
          kevesebb nem válaszai döntik el.
        </p>
        <ul className="flex flex-col gap-3">
          {esemeny.idopontok.map((idopont) => (
            <IdopontCard
              key={idopont.id}
              esemenyId={esemeny.id}
              idopont={idopont}
              answerable={answerable}
            />
          ))}
        </ul>
      </section>
    </>
  );
}

const valaszok: Valasz[] = ["igen", "talan", "nem"];

const selectedClass: Record<Valasz, string> = {
  igen: "border-green-600 bg-green-600 text-white",
  talan: "border-amber-500 bg-amber-500 text-white",
  nem: "border-red-600 bg-red-600 text-white",
};

function IdopontCard({
  esemenyId,
  idopont,
  answerable,
}: {
  esemenyId: string;
  idopont: Idopont;
  answerable: boolean;
}) {
  return (
    <li className={`${cardClass} flex flex-col gap-3`}>
      <div className="flex items-start justify-between gap-4">
        <span className="font-medium">
          {formatIdopont(idopont.start, idopont.end)}
        </span>
        <span className="shrink-0 text-lg font-semibold" title="Helyezés">
          <span className="sr-only">Helyezés: </span>
          {idopont.helyezes === null ? "–" : `${idopont.helyezes}.`}
        </span>
      </div>
      <p className="text-sm text-zinc-600 dark:text-zinc-400">
        Igen: {idopont.igen} · Talán: {idopont.talan} · Nem: {idopont.nem}
      </p>
      {idopont.visszajelzesek.length > 0 ? (
        <ul className="flex flex-col gap-1 text-sm">
          {valaszok.map((valasz) => {
            const names = idopont.visszajelzesek
              .filter((v) => v.valasz === valasz)
              .map((v) => (v.isTag ? v.name : `${v.name} (nem tag)`));
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
        {answerable
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
    </li>
  );
}
