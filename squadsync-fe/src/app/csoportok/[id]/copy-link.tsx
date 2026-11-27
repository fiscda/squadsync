"use client";

import { useState } from "react";
import { secondaryButtonClass } from "@/lib/ui";

/** A read-only link with a button that copies it to the clipboard. */
export function CopyLink({ url }: { url: string }) {
  const [state, setState] = useState<"idle" | "copied" | "failed">("idle");

  async function copy() {
    try {
      await navigator.clipboard.writeText(url);
      setState("copied");
    } catch {
      // No clipboard access (e.g. plain HTTP): the link is selectable.
      setState("failed");
    }
    setTimeout(() => setState("idle"), 2500);
  }

  return (
    <div className="flex min-w-0 flex-1 items-center gap-2">
      <input
        readOnly
        value={url}
        aria-label="Meghívó link"
        onFocus={(e) => e.currentTarget.select()}
        className="min-w-0 flex-1 rounded-full border border-zinc-300 bg-transparent px-3 py-2 font-mono text-sm dark:border-zinc-700"
      />
      <button type="button" onClick={copy} className={secondaryButtonClass}>
        {state === "copied"
          ? "Másolva!"
          : state === "failed"
            ? "Nem sikerült"
            : "Másolás"}
      </button>
    </div>
  );
}
