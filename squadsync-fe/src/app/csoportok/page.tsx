import Link from "next/link";
import { Suspense } from "react";
import { getMyCsoportok } from "@/lib/csoport";
import { cardClass, linkClass } from "@/lib/ui";
import { CreateCsoportForm } from "./create-csoport-form";

export default function CsoportokPage() {
  return (
    <main className="mx-auto flex w-full max-w-xl flex-1 flex-col gap-8 px-4 py-12">
      <div className="flex flex-col gap-2">
        <Link href="/" className={`${linkClass} text-sm`}>
          ← Főoldal
        </Link>
        <h1 className="text-3xl font-semibold tracking-tight">Csoportjaim</h1>
      </div>
      <Suspense fallback={<p className="text-zinc-500">Betöltés…</p>}>
        <Csoportok />
      </Suspense>
    </main>
  );
}

// Reads the session, so it streams in behind the Suspense boundary.
async function Csoportok() {
  const csoportok = await getMyCsoportok();

  return (
    <>
      {csoportok.length === 0 ? (
        <p className="text-zinc-600 dark:text-zinc-400">
          Még egyetlen csoportnak sem vagy tagja. Hozz létre egyet, vagy kérj
          meghívó linket egy barátodtól.
        </p>
      ) : (
        <ul className="flex flex-col gap-3">
          {csoportok.map((csoport) => (
            <li key={csoport.id}>
              <Link
                href={`/csoportok/${csoport.id}`}
                className={`${cardClass} flex items-center justify-between gap-4 transition-colors hover:bg-zinc-50 dark:hover:bg-zinc-900`}
              >
                <span className="font-medium">{csoport.name}</span>
                <span className="text-sm text-zinc-500">
                  {csoport.admin ? "Admin · " : ""}
                  {csoport.tagCount} tag
                </span>
              </Link>
            </li>
          ))}
        </ul>
      )}
      <CreateCsoportForm />
    </>
  );
}
