import Link from "next/link";
import { Suspense } from "react";
import { getMyDiscordAccounts } from "@/lib/discord-link";
import { getMyGoogleNaptar } from "@/lib/google-naptar";
import {
  buttonClass,
  cardClass,
  formatDate,
  linkClass,
  secondaryButtonClass,
} from "@/lib/ui";
import {
  disconnectGoogleNaptarAction,
  unlinkDiscordAccountAction,
} from "./actions";

export default function FiokPage({ searchParams }: PageProps<"/fiok">) {
  return (
    <main className="mx-auto flex w-full max-w-xl flex-1 flex-col gap-8 px-4 py-12">
      <div className="flex flex-col gap-2">
        <Link href="/" className={`${linkClass} text-sm`}>
          ← Főoldal
        </Link>
        <h1 className="text-3xl font-semibold tracking-tight">Fiókom</h1>
        <Link href="/raeres" className={`${linkClass} text-sm`}>
          Ráérésem megadása →
        </Link>
      </div>
      <Suspense fallback={<p className="text-zinc-500">Betöltés…</p>}>
        {searchParams.then(({ kapcsolas }) => (
          <DiscordFiokok kapcsolas={first(kapcsolas)} />
        ))}
      </Suspense>
      <Suspense fallback={<p className="text-zinc-500">Betöltés…</p>}>
        {searchParams.then(({ google }) => (
          <GoogleNaptarSection google={first(google)} />
        ))}
      </Suspense>
    </main>
  );
}

function first(v: string | string[] | undefined): string | undefined {
  return Array.isArray(v) ? v[0] : v;
}

// The outcome of a linking flow, from the callback route's `?kapcsolas=`.
const kapcsolasMessages: Record<string, { text: string; ok: boolean }> = {
  ok: { text: "Sikerült: a fiókot hozzákapcsoltuk a tiédhez.", ok: true },
  "mar-a-tied": {
    text: "Ez a Discord fiók már a tiéd, nem változott semmi.",
    ok: true,
  },
  foglalt: { text: "Ez a fiók már használatban van.", ok: false },
  megszakitva: { text: "A kapcsolást megszakítottad.", ok: false },
  "nem-elerheto": {
    text: "A fiókkapcsolás most nem érhető el, mert nincs beállítva. Szólj az oldal gazdájának.",
    ok: false,
  },
  hiba: { text: "Nem sikerült a kapcsolás. Próbáld újra.", ok: false },
};

// Reads the session, so it streams in behind the Suspense boundary.
async function DiscordFiokok({ kapcsolas }: { kapcsolas?: string }) {
  const accounts = await getMyDiscordAccounts();
  const message = kapcsolas ? kapcsolasMessages[kapcsolas] : undefined;

  return (
    <section className="flex flex-col gap-4">
      <h2 className="text-xl font-semibold">Discord fiókjaim</h2>
      {message && (
        <output
          className={
            message.ok
              ? "text-green-700 dark:text-green-400"
              : "text-red-700 dark:text-red-400"
          }
        >
          {message.text}
        </output>
      )}
      <ul className="flex flex-col gap-2">
        {accounts.map((account) => (
          <li
            key={account.discordId}
            className={`${cardClass} flex items-center justify-between gap-4`}
          >
            <div className="flex flex-col">
              <span className="font-medium">
                {account.name || "Discord fiók"}
              </span>
              <span className="text-sm text-zinc-500">
                {account.main
                  ? "Fő fiók: ezzel lépsz be a weboldalra"
                  : `Kapcsolva ${formatDate(account.createdAt)} óta`}
              </span>
            </div>
            {!account.main && (
              <form
                action={unlinkDiscordAccountAction.bind(
                  null,
                  account.discordId,
                )}
              >
                <button type="submit" className={secondaryButtonClass}>
                  Leválasztás
                </button>
              </form>
            )}
          </li>
        ))}
      </ul>
      <p className="text-sm text-zinc-600 dark:text-zinc-400">
        A kapcsolt fiókjaidról is válaszolhatsz a botnak, és minden üzenetet
        mindegyikre megkapsz. A weboldalra csak a fő fiókoddal léphetsz be. A
        Discord oldalán a „Nem te vagy?” linkkel válthatsz át a másik fiókodra.
      </p>
      <form method="post" action="/fiok/discord/kapcsolas">
        <button type="submit" className={buttonClass}>
          Másik Discord fiók kapcsolása
        </button>
      </form>
    </section>
  );
}

// The outcome of connecting Google Calendar, from the callback's `?google=`.
const googleMessages: Record<string, { text: string; ok: boolean }> = {
  ok: {
    text: "Sikerült: a Google Naptárad össze van kapcsolva. Az eseményeid hamarosan megjelennek.",
    ok: true,
  },
  megszakitva: { text: "Az összekapcsolást megszakítottad.", ok: false },
  "nem-elerheto": {
    text: "A Google Naptár most nem érhető el, mert nincs beállítva. Szólj az oldal gazdájának.",
    ok: false,
  },
  hiba: { text: "Nem sikerült az összekapcsolás. Próbáld újra.", ok: false },
};

// Reads the session, so it streams in behind the Suspense boundary.
async function GoogleNaptarSection({ google }: { google?: string }) {
  const naptar = await getMyGoogleNaptar();
  const message = google ? googleMessages[google] : undefined;
  const connectButton = (label: string) => (
    <form method="post" action="/fiok/google/kapcsolas">
      <button type="submit" className={buttonClass}>
        {label}
      </button>
    </form>
  );

  return (
    <section className="flex flex-col gap-4">
      <h2 className="text-xl font-semibold">Google Naptár</h2>
      {message && (
        <output
          className={
            message.ok
              ? "text-green-700 dark:text-green-400"
              : "text-red-700 dark:text-red-400"
          }
        >
          {message.text}
        </output>
      )}
      <p className="text-sm text-zinc-600 dark:text-zinc-400">
        A véglegesített eseményeid, amelyeken részt veszel, bekerülnek egy külön
        „Squad Sync” naptárba a Google fiókodban. Csak ezt az egy naptárat
        használjuk: a többi naptáradhoz nem férünk hozzá. Egy kattintással
        elrejtheted a Google Naptárban.
      </p>
      {naptar.state === "not_configured" && (
        <p className="text-sm text-zinc-500">
          A Google Naptár szinkron nincs beállítva ezen az oldalon. Szólj az
          oldal gazdájának.
        </p>
      )}
      {naptar.state === "disconnected" &&
        connectButton("Google Naptár összekapcsolása")}
      {(naptar.state === "connected" || naptar.state === "broken") && (
        <div className={`${cardClass} flex flex-col gap-3`}>
          {naptar.state === "connected" ? (
            <span className="font-medium text-green-700 dark:text-green-400">
              Összekapcsolva
              {naptar.connectedAt && ` ${formatDate(naptar.connectedAt)} óta`}
            </span>
          ) : (
            <span className="font-medium text-red-700 dark:text-red-400">
              A kapcsolat megszakadt, újra kell kapcsolni: a Google
              visszautasította a hozzáférést.
            </span>
          )}
          <div className="flex flex-wrap gap-2">
            {naptar.state === "broken" && connectButton("Újrakapcsolás")}
            <form action={disconnectGoogleNaptarAction}>
              <button type="submit" className={secondaryButtonClass}>
                Leválasztás
              </button>
            </form>
          </div>
          <p className="text-sm text-zinc-600 dark:text-zinc-400">
            Leválasztás után nem frissítjük tovább a naptárat. A „Squad Sync”
            naptár a meglévő eseményekkel a Google fiókodban marad; ott
            törölheted. Ha leválasztás nélkül törlöd, a következő változáskor
            újra létrehozzuk.
          </p>
        </div>
      )}
    </section>
  );
}
