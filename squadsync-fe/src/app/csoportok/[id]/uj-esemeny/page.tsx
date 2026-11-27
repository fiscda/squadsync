import Link from "next/link";
import { notFound } from "next/navigation";
import { Suspense } from "react";
import { getCsoport } from "@/lib/csoport";
import { linkClass } from "@/lib/ui";
import { EsemenyForm } from "./esemeny-form";

export default function UjEsemenyPage({
  params,
}: PageProps<"/csoportok/[id]/uj-esemeny">) {
  return (
    <main className="mx-auto flex w-full max-w-xl flex-1 flex-col gap-8 px-4 py-12">
      <Suspense fallback={<p className="text-zinc-500">Betöltés…</p>}>
        {params.then(({ id }) => (
          <UjEsemeny csoportId={id} />
        ))}
      </Suspense>
    </main>
  );
}

// Reads the session and the route params, so it streams in behind Suspense.
async function UjEsemeny({ csoportId }: { csoportId: string }) {
  const csoport = await getCsoport(csoportId);
  if (!csoport) {
    notFound();
  }
  return (
    <>
      <div className="flex flex-col gap-2">
        <Link
          href={`/csoportok/${csoport.id}`}
          className={`${linkClass} text-sm`}
        >
          ← {csoport.name}
        </Link>
        <h1 className="text-3xl font-semibold tracking-tight">Új esemény</h1>
      </div>
      <EsemenyForm csoportId={csoport.id} />
    </>
  );
}
