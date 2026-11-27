import Link from "next/link";
import { Suspense } from "react";
import { getCurrentFelhasznalo } from "@/lib/felhasznalo";
import { loginErrorMessage } from "@/lib/login-error";
import { buttonClass, secondaryButtonClass } from "@/lib/ui";
import { retryFirstLogin, signInWithDiscord, signOut } from "./actions";

export default function Home({ searchParams }: PageProps<"/">) {
  return (
    <main className="mx-auto flex w-full max-w-xl flex-1 flex-col items-center justify-center gap-8 px-4 py-16 text-center">
      <h1 className="text-4xl font-semibold tracking-tight">Squad Sync</h1>
      <Suspense>
        {searchParams.then((params) => (
          <LoginError
            error={first(params.error)}
            description={first(params.error_description)}
          />
        ))}
      </Suspense>
      <Suspense fallback={<p className="text-zinc-500">Betöltés…</p>}>
        <Greeting />
      </Suspense>
    </main>
  );
}

function first(v: string | string[] | undefined): string | undefined {
  return Array.isArray(v) ? v[0] : v;
}

// A refused login comes back here with ?error= (see discordLoginUrl). The
// text never comes from the URL (see lib/login-error.ts).
async function LoginError({
  error,
  description,
}: {
  error?: string;
  description?: string;
}) {
  const message = await loginErrorMessage(error, description);
  if (!message) {
    return null;
  }
  return (
    <p role="alert" className="text-lg text-red-700 dark:text-red-400">
      {message}
    </p>
  );
}

// Reads the session, so it streams in behind the Suspense boundary.
async function Greeting() {
  // Errors (e.g. Go is down) are shown by error.tsx.
  const current = await getCurrentFelhasznalo();

  if (current.status === "anonymous") {
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

  if (current.status === "missing") {
    return (
      <div className="flex flex-col items-center gap-4">
        <p className="text-lg">
          Beléptél, de a profilodat még nem sikerült létrehozni.
        </p>
        <form action={retryFirstLogin}>
          <button type="submit" className={buttonClass}>
            Újrapróbálom
          </button>
        </form>
      </div>
    );
  }

  const { felhasznalo } = current;
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
      <Link href="/csoportok" className={buttonClass}>
        Csoportjaim
      </Link>
      <Link href="/raeres" className={secondaryButtonClass}>
        Ráérésem
      </Link>
      <Link href="/fiok" className={secondaryButtonClass}>
        Fiókom
      </Link>
      <form action={signOut}>
        <button type="submit" className={buttonClass}>
          Kilépés
        </button>
      </form>
    </div>
  );
}
