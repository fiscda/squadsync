import Link from "next/link";
import { notFound } from "next/navigation";
import { Suspense } from "react";
import type { CsoportDetail } from "@/lib/backend";
import { getCsoport, getCsoportMeghivok } from "@/lib/csoport";
import {
  buttonClass,
  cardClass,
  formatDate,
  linkClass,
  secondaryButtonClass,
} from "@/lib/ui";
import {
  createMeghivoAction,
  revokeMeghivoAction,
  unlinkCsatornaAction,
} from "../actions";
import { CopyLink } from "./copy-link";

export default function CsoportPage({ params }: PageProps<"/csoportok/[id]">) {
  return (
    <main className="mx-auto flex w-full max-w-xl flex-1 flex-col gap-8 px-4 py-12">
      <Link href="/csoportok" className={`${linkClass} text-sm`}>
        ← Csoportjaim
      </Link>
      <Suspense fallback={<p className="text-zinc-500">Betöltés…</p>}>
        {params.then(({ id }) => (
          <Csoport csoportId={id} />
        ))}
      </Suspense>
    </main>
  );
}

// Reads the session and the route params, so it streams in behind Suspense.
async function Csoport({ csoportId }: { csoportId: string }) {
  const csoport = await getCsoport(csoportId);
  if (!csoport) {
    // Not a Tag, or no such Csoport: the same answer either way.
    notFound();
  }

  return (
    <>
      <div className="flex flex-col gap-1">
        <h1 className="text-3xl font-semibold tracking-tight">
          {csoport.name}
        </h1>
        <p className="text-sm text-zinc-500">
          Létrehozva: {formatDate(csoport.createdAt)}
        </p>
      </div>

      <section className="flex flex-col gap-3">
        <h2 className="text-xl font-semibold">Tagok ({csoport.tags.length})</h2>
        <ul className="flex flex-col gap-2">
          {csoport.tags.map((tag) => (
            <li
              key={tag.felhasznaloId}
              className={`${cardClass} flex items-center justify-between gap-4`}
            >
              <span className="font-medium">{tag.name}</span>
              <span className="text-sm text-zinc-500">
                {tag.admin ? "Admin" : `Tag ${formatDate(tag.createdAt)} óta`}
              </span>
            </li>
          ))}
        </ul>
      </section>

      <CsatornaSection csoport={csoport} />

      <MeghivoSection csoportId={csoport.id} />
    </>
  );
}

function CsatornaSection({ csoport }: { csoport: CsoportDetail }) {
  const { csatorna } = csoport;
  return (
    <section className="flex flex-col gap-3">
      <h2 className="text-xl font-semibold">Discord-csatorna</h2>
      {csatorna ? (
        <div
          className={`${cardClass} flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between`}
        >
          <div className="flex flex-col gap-1">
            <span className="font-medium">
              {csatorna.channelName
                ? `#${csatorna.channelName}`
                : `Csatorna azonosító: ${csatorna.channelId}`}
            </span>
            <span className="text-xs text-zinc-500">
              Összekapcsolva: {formatDate(csatorna.linkedAt)}
            </span>
          </div>
          {csoport.admin && (
            <form action={unlinkCsatornaAction.bind(null, csoport.id)}>
              <button type="submit" className={secondaryButtonClass}>
                Leválasztás
              </button>
            </form>
          )}
        </div>
      ) : (
        <p className="text-zinc-500">
          Nincs Discord-csatorna összekapcsolva, ezért minden értesítés
          üzenetben (DM) megy.
        </p>
      )}
      {csoport.admin && (
        <p className="text-sm text-zinc-600 dark:text-zinc-400">
          Csatornát a Discordban kapcsolhatsz: írd be a{" "}
          <code className="font-mono">/csatorna</code> parancsot abban a
          csatornában, ahová a bejelentéseket szeretnéd, és válaszd ki ezt a
          csoportot. Újra futtatva lecseréli a csatornát.
        </p>
      )}
    </section>
  );
}

async function MeghivoSection({ csoportId }: { csoportId: string }) {
  const meghivok = (await getCsoportMeghivok(csoportId)) ?? [];
  // The public address of the web app, so the copied link works anywhere.
  const publicUrl = process.env.BETTER_AUTH_URL;
  if (!publicUrl) {
    throw new Error("BETTER_AUTH_URL is not set (see .env.example)");
  }
  const baseUrl = publicUrl.replace(/\/$/, "");

  return (
    <section className="flex flex-col gap-3">
      <h2 className="text-xl font-semibold">Meghívó linkek</h2>
      <p className="text-sm text-zinc-600 dark:text-zinc-400">
        Aki megnyitja a linket és belép, tagja lesz a csoportnak. A link nem jár
        le, amíg vissza nem vonod.
      </p>
      {meghivok.length === 0 ? (
        <p className="text-zinc-500">Még nincs érvényes meghívó link.</p>
      ) : (
        <ul className="flex flex-col gap-3">
          {meghivok.map((meghivo) => (
            <li key={meghivo.id} className={`${cardClass} flex flex-col gap-3`}>
              <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
                <CopyLink
                  url={`${baseUrl}/meghivo/${encodeURIComponent(meghivo.token)}`}
                />
                <form
                  action={revokeMeghivoAction.bind(null, csoportId, meghivo.id)}
                >
                  <button type="submit" className={secondaryButtonClass}>
                    Visszavonás
                  </button>
                </form>
              </div>
              <p className="text-xs text-zinc-500">
                Létrehozva: {formatDate(meghivo.createdAt)}
              </p>
            </li>
          ))}
        </ul>
      )}
      <form action={createMeghivoAction.bind(null, csoportId)}>
        <button type="submit" className={buttonClass}>
          Új meghívó link
        </button>
      </form>
    </section>
  );
}
