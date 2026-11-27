import { Suspense } from "react";
import { cardClass } from "@/lib/ui";
import { setKiegeszitoAction } from "./actions";
import type { KiegeszitoPanelProps } from "./panel-props";
import { kiegeszitoRegistry } from "./registry";

/**
 * The Kiegészítők of an Esemény: the Szervező's on/off toggles, then the
 * panel of each one that is on (see registry.tsx).
 */
export function KiegeszitokSection(props: KiegeszitoPanelProps) {
  const { esemeny, isSzervezo } = props;
  const enabled = esemeny.kiegeszitok.filter((k) => k.enabled);
  const toggles = isSzervezo
    ? esemeny.kiegeszitok.filter((k) => k.switchable)
    : [];
  if (enabled.length === 0 && toggles.length === 0) {
    return null;
  }
  return (
    <section className="flex flex-col gap-4">
      <h2 className="text-xl font-semibold">Kiegészítők</h2>
      {toggles.length > 0 ? (
        <div className="flex flex-col gap-2">
          <p className="text-sm text-zinc-600 dark:text-zinc-400">
            Szervezőként kapcsold be, amire szükség van. Kikapcsoláskor semmi
            nem vész el, csak elrejtődik.
          </p>
          <ul className="flex flex-wrap gap-2">
            {toggles.map((k) => {
              const entry = kiegeszitoRegistry[k.kind];
              return (
                <li key={k.kind}>
                  <form
                    action={setKiegeszitoAction.bind(
                      null,
                      esemeny.id,
                      k.kind,
                      !k.enabled,
                    )}
                  >
                    <button
                      type="submit"
                      aria-pressed={k.enabled}
                      className={`rounded-full border px-4 py-1.5 text-sm font-medium transition-opacity hover:opacity-80 ${
                        k.enabled
                          ? "border-green-600 bg-green-600 text-white"
                          : "border-zinc-300 dark:border-zinc-700"
                      }`}
                    >
                      {entry.icon} {entry.label}: {k.enabled ? "be" : "ki"}
                    </button>
                  </form>
                </li>
              );
            })}
          </ul>
        </div>
      ) : null}
      {enabled.map((k) => {
        const { Panel, icon, label } = kiegeszitoRegistry[k.kind];
        if (Panel === null) {
          return null;
        }
        return (
          <section
            key={k.kind}
            className={`${cardClass} flex flex-col gap-3`}
            aria-label={label}
          >
            <h3 className="text-lg font-semibold">
              {icon} {label}
            </h3>
            <Suspense
              fallback={<p className="text-sm text-zinc-500">Betöltés…</p>}
            >
              <Panel {...props} />
            </Suspense>
          </section>
        );
      })}
    </section>
  );
}
