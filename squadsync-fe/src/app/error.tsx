"use client";

import { useEffect } from "react";

export default function ErrorPage({
  error,
  retry,
}: {
  error: unknown;
  retry: () => void;
}) {
  useEffect(() => {
    console.error(error);
  }, [error]);

  return (
    <main className="mx-auto flex w-full max-w-xl flex-1 flex-col items-center justify-center gap-6 px-4 py-16 text-center">
      <p className="text-lg">
        Most nem sikerült betölteni az oldalt. Próbáld újra kicsit később.
      </p>
      <button
        type="button"
        onClick={() => retry()}
        className="rounded-full bg-foreground px-5 py-3 font-medium text-background transition-opacity hover:opacity-80"
      >
        Újrapróbálom
      </button>
    </main>
  );
}
