import Link from "next/link";
import { redirect } from "next/navigation";
import { Suspense } from "react";
import { getCurrentFelhasznalo } from "@/lib/felhasznalo";
import { buttonClass, linkClass } from "@/lib/ui";
import {
  getEsemenyMeghivoPreview,
  getMemberEsemenyOfMeghivo,
} from "@/lib/vendeg";
import { joinEsemenyAction, signInForEsemenyMeghivo } from "../actions";

export default function EsemenyMeghivoPage({
  params,
}: PageProps<"/esemeny-meghivo/[token]">) {
  return (
    <main className="mx-auto flex w-full max-w-xl flex-1 flex-col items-center justify-center gap-6 px-4 py-16 text-center">
      <Suspense fallback={<p className="text-zinc-500">Betöltés…</p>}>
        {params.then(({ token }) => (
          <EsemenyMeghivo token={token} />
        ))}
      </Suspense>
    </main>
  );
}

// Reads the session and the route params, so it streams in behind Suspense.
async function EsemenyMeghivo({ token }: { token: string }) {
  // A Tag or a Vendég of that Esemény goes straight in, even if the link no
  // longer works.
  const memberEsemenyId = await getMemberEsemenyOfMeghivo(token);
  if (memberEsemenyId !== null) {
    redirect(`/esemenyek/${memberEsemenyId}`);
  }

  const preview = await getEsemenyMeghivoPreview(token);

  if (preview.status === "revoked") {
    return (
      <Refusal>
        Ez az esemény-meghívó link már nem érvényes, mert visszavonták. Kérj egy
        újat attól, aki küldte.
      </Refusal>
    );
  }
  if (preview.status === "expired") {
    return (
      <Refusal>
        Ez az esemény-meghívó link már nem érvényes, mert az esemény lezajlott
        vagy elmaradt.
      </Refusal>
    );
  }
  if (preview.status === "not-found") {
    return (
      <Refusal>
        Ez az esemény-meghívó link nem létezik. Ellenőrizd, hogy a teljes linket
        másoltad-e be.
      </Refusal>
    );
  }

  const { esemeny } = preview;
  const current = await getCurrentFelhasznalo();

  return (
    <>
      <h1 className="text-3xl font-semibold tracking-tight">
        Meghívtak egy eseményre
      </h1>
      <p className="text-lg text-zinc-600 dark:text-zinc-400">
        <strong className="text-foreground">{esemeny.title}</strong>
        <br />
        Szervező: {esemeny.szervezoName}
      </p>
      <p className="text-sm text-zinc-500">
        Vendégként csak ezt az eseményt látod: válaszolhatsz az időpontokra, és
        Discordon emlékeztetőt kapsz róla.
      </p>
      {current.status === "anonymous" ? (
        <form action={signInForEsemenyMeghivo.bind(null, token)}>
          <p className="mb-4 text-zinc-600 dark:text-zinc-400">
            Lépj be a Discord-fiókoddal, és azonnal vendég leszel.
          </p>
          <button type="submit" className={buttonClass}>
            Belépés Discorddal és csatlakozás
          </button>
        </form>
      ) : (
        <form action={joinEsemenyAction.bind(null, token)}>
          <button type="submit" className={buttonClass}>
            Csatlakozás vendégként
          </button>
        </form>
      )}
    </>
  );
}

function Refusal({ children }: { children: React.ReactNode }) {
  return (
    <>
      <p role="alert" className="text-lg">
        {children}
      </p>
      <Link href="/" className={linkClass}>
        Vissza a főoldalra
      </Link>
    </>
  );
}
