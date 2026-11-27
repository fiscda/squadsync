import { Suspense } from "react";
import { getCurrentFelhasznalo } from "@/lib/felhasznalo";
import { signInWithDiscord, signOut } from "./actions";

const buttonClass =
  "rounded-full bg-foreground px-5 py-3 font-medium text-background transition-opacity hover:opacity-80";

export default function Home() {
  return (
    <main className="mx-auto flex w-full max-w-xl flex-1 flex-col items-center justify-center gap-8 px-4 py-16 text-center">
      <h1 className="text-4xl font-semibold tracking-tight">Squad Sync</h1>
      <Suspense fallback={<p className="text-zinc-500">Betöltés…</p>}>
        <Udvozles />
      </Suspense>
    </main>
  );
}

// Reads the session, so it streams in behind the Suspense boundary.
async function Udvozles() {
  // Errors (e.g. Go is down) are shown by error.tsx.
  const felhasznalo = await getCurrentFelhasznalo();

  if (!felhasznalo) {
    return (
      <div className="flex flex-col items-center gap-4">
        <p className="text-lg text-zinc-600 dark:text-zinc-400">
          Lépj be a Discord-fiókoddal, hogy lásd a csoportjaidat.
        </p>
        <form action={signInWithDiscord}>
          <button type="submit" className={buttonClass}>
            Belépés Discorddal
          </button>
        </form>
      </div>
    );
  }

  const since = new Intl.DateTimeFormat("hu-HU", {
    dateStyle: "long",
    timeZone: "Europe/Budapest",
  }).format(new Date(felhasznalo.createdAt));

  return (
    <div className="flex flex-col items-center gap-4">
      <p className="text-2xl">Szia, {felhasznalo.name}!</p>
      <p className="text-zinc-600 dark:text-zinc-400">
        {since} óta használod a Squad Syncet.
      </p>
      <form action={signOut}>
        <button type="submit" className={buttonClass}>
          Kilépés
        </button>
      </form>
    </div>
  );
}
