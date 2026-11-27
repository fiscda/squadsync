import Link from "next/link";
import { getEsemenyRaeres } from "@/lib/raeres";
import { linkClass } from "@/lib/ui";

/**
 * "X/Y szabad aznap" for one Időpont: how many Tags are free on every day it
 * covers. While the Időpont can be answered it also tells what the caller's
 * own Ráérés says about those days. The read is cached per request, so
 * every Időpont shares one call to Go.
 */
export async function IdopontRaeresLine({
  esemenyId,
  idopontId,
  answerable,
}: {
  esemenyId: string;
  idopontId: string;
  answerable: boolean;
}) {
  const raeres = (await getEsemenyRaeres(esemenyId))?.find(
    (r) => r.idopontId === idopontId,
  );
  if (!raeres) {
    return null;
  }
  return (
    <div className="flex flex-col gap-1 text-sm">
      <p className="text-zinc-600 dark:text-zinc-400">
        🗓 {raeres.szabad}/{raeres.osszes} szabad aznap
      </p>
      {answerable && raeres.sajat !== null ? (
        <p
          className={
            raeres.sajat === "szabad"
              ? "text-green-700 dark:text-green-400"
              : "text-red-700 dark:text-red-400"
          }
        >
          A Ráérésed szerint ezeken a napokon{" "}
          {raeres.sajat === "szabad" ? "szabad vagy" : "foglalt vagy"}.{" "}
          <Link href="/raeres" className={linkClass}>
            Ráérésem
          </Link>
        </p>
      ) : null}
    </div>
  );
}
