import Link from "next/link";
import { linkClass } from "@/lib/ui";

export default function NotFound() {
  return (
    <main className="mx-auto flex w-full max-w-xl flex-1 flex-col items-center justify-center gap-4 px-4 py-16 text-center">
      <p className="text-lg">Ez az oldal nem található.</p>
      <Link href="/" className={linkClass}>
        Vissza a főoldalra
      </Link>
    </main>
  );
}
