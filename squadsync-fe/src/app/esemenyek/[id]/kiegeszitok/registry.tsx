import type { ReactNode } from "react";
import type { KiegeszitoKind } from "@/lib/backend";
import { AutokPanel } from "./autok/panel";
import { FelszerelesPanel } from "./felszereles/panel";
import type { KiegeszitoPanelProps } from "./panel-props";

/**
 * Every Kiegészítő as the Esemény page shows it. To add one, write its panel
 * (kiegeszitok/<kind>/panel.tsx, a server component that fetches its own
 * data) and set Panel on its entry below. hibak words the error codes its
 * Go routes answer with, shown above the page as ?hiba=<code>; prefix them
 * with the kind ("felszereles_too_much") so they never clash.
 *
 * The toggles list only the Kiegészítők Go reports (the built ones); one
 * without a Panel shows nothing when switched on.
 */
export const kiegeszitoRegistry: Record<
  KiegeszitoKind,
  {
    label: string;
    icon: string;
    Panel:
      | ((props: KiegeszitoPanelProps) => ReactNode | Promise<ReactNode>)
      | null;
    hibak?: Record<string, string>;
  }
> = {
  koltsegek: { label: "Költségek", icon: "🧾", Panel: null },
  autok: {
    label: "Autók",
    icon: "🚗",
    Panel: AutokPanel,
    hibak: {
      autok_full: "Ebben az autóban már nincs szabad hely.",
      autok_already_sofor: "Már ajánlottál egy autót.",
      autok_sofor_cant_ride:
        "A sofőr nem ülhet másik autóba, és nem lehet „saját úton” sem.",
      autok_already_seated:
        "Már van helyed egy autóban. Előbb szállj ki, utána szállhatsz be máshova.",
      autok_seats_below_utasok:
        "Az autóban több utas ül, mint ahány helyet megadtál.",
    },
  },
  szallas: { label: "Szállás", icon: "🛏", Panel: null },
  felszereles: {
    label: "Felszerelés",
    icon: "🎒",
    Panel: FelszerelesPanel,
    hibak: {
      felszereles_too_much:
        "Ennyit nem vállalhatsz: többet, mint ami még hiányzik.",
      felszereles_mindenki_hozza:
        "Ezt mindenki maga hozza, nem lehet vállalni.",
      felszereles_tetel_claimed:
        "Ez a módosítás ütközik a vállalásokkal: a mennyiség nem lehet kevesebb a vállaltnál, és vállalt tétel nem lehet „mindenki hozza”.",
    },
  },
  kaja: { label: "Kaja", icon: "🍲", Panel: null },
  lejatszasi_lista: { label: "Lejátszási lista", icon: "🎵", Panel: null },
};

export function isKiegeszitoKind(value: unknown): value is KiegeszitoKind {
  return typeof value === "string" && Object.hasOwn(kiegeszitoRegistry, value);
}

/** The framework's error codes (see httpapi/kiegeszito.go). */
const frameworkHibak: Record<string, string> = {
  kiegeszito_off: "Ez a kiegészítő közben ki lett kapcsolva.",
  kiegeszito_locked:
    "Elmaradt vagy lezajlott eseménynél már nem lehet kiegészítőt be- vagy kikapcsolni.",
  kiegeszito_in_use:
    "Ezt a kiegészítőt most nem lehet kikapcsolni, mert használatban van.",
  setup_closed: "Elmaradt vagy lezajlott eseménynél már nem lehet módosítani.",
  claiming_closed:
    "Vállalni csak véglegesített eseménynél lehet. Újranyitott egyeztetés alatt a vállalások be vannak fagyasztva.",
  not_resztvevo:
    "Vállalni csak résztvevőként lehet: mondj igent a végleges időpontra.",
  not_permitted: "Ehhez nincs jogod.",
  ervenytelen: "Valamelyik mező hibás. Ellenőrizd az adatokat.",
  nincs: "Ez közben megszűnt. Az oldal már a mostani állapotot mutatja.",
};

/** The notice for a Kiegészítő error code, or null if it isn't one. */
export function kiegeszitoHibaText(code: unknown): string | null {
  if (typeof code !== "string") {
    return null;
  }
  if (Object.hasOwn(frameworkHibak, code)) {
    return frameworkHibak[code];
  }
  for (const entry of Object.values(kiegeszitoRegistry)) {
    if (entry.hibak && Object.hasOwn(entry.hibak, code)) {
      return entry.hibak[code];
    }
  }
  return null;
}
