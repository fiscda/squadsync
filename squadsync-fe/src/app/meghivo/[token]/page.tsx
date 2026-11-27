import Link from "next/link";
import { redirect } from "next/navigation";
import { Suspense } from "react";
import { getExistingTagCsoportId, getMeghivoPreview } from "@/lib/csoport";
import { getCurrentFelhasznalo } from "@/lib/felhasznalo";
import { buttonClass, linkClass } from "@/lib/ui";
import { joinAction, signInForMeghivo } from "../actions";

export default function MeghivoPage({ params }: PageProps<"/meghivo/[token]">) {
  return (
    <main className="mx-auto flex w-full max-w-xl flex-1 flex-col items-center justify-center gap-6 px-4 py-16 text-center">
      <Suspense fallback={<p className="text-zinc-500">Betöltés…</p>}>
        {params.then(({ token }) => (
          <Meghivo token={token} />
        ))}
      </Suspense>
    </main>
  );
}

// Reads the session and the route params, so it streams in behind Suspense.
async function Meghivo({ token }: { token: string }) {
  // A Tag of that Csoport goes straight in, even if the link was revoked.
  const tagCsoportId = await getExistingTagCsoportId(token);
  if (tagCsoportId !== null) {
    redirect(`/csoportok/${tagCsoportId}`);
  }

  const preview = await getMeghivoPreview(token);

  if (preview.status === "revoked") {
    return (
      <Refusal>
        Ez a meghívó link már nem érvényes, mert visszavonták. Kérj egy újat
        attól, aki küldte.
      </Refusal>
    );
  }
  if (preview.status === "not-found") {
    return (
      <Refusal>
        Ez a meghívó link nem létezik. Ellenőrizd, hogy a teljes linket
        másoltad-e be.
      </Refusal>
    );
  }

  const { csoport } = preview;
  const current = await getCurrentFelhasznalo();

  return (
    <>
      <h1 className="text-3xl font-semibold tracking-tight">Meghívót kaptál</h1>
      <p className="text-lg text-zinc-600 dark:text-zinc-400">
        A(z) <strong className="text-foreground">{csoport.name}</strong>{" "}
        csoportba hívtak meg.
      </p>
      {current.status === "anonymous" ? (
        <form action={signInForMeghivo.bind(null, token)}>
          <p className="mb-4 text-zinc-600 dark:text-zinc-400">
            Lépj be a Discord-fiókoddal, és azonnal tag leszel.
          </p>
          <button type="submit" className={buttonClass}>
            Belépés Discorddal és csatlakozás
          </button>
        </form>
      ) : (
        <form action={joinAction.bind(null, token)}>
          <button type="submit" className={buttonClass}>
            Csatlakozás
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
