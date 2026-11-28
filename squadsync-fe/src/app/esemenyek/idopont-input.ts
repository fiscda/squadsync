import type { EsemenyInput } from "@/lib/backend";

export type IdopontInputs = EsemenyInput["idopontok"];

/**
 * Reads the Időpontok that the IdopontRows form fields sent: each row has
 * an egeszNapos flag and a start and end date and time, all Budapest local
 * time. The checks only give friendly Hungarian messages: Go has the final
 * say. Used by every action that takes Időpontok (proposing, adding).
 */
export function idopontokFromForm(
  formData: FormData,
): { idopontok: IdopontInputs } | { error: string } {
  const all = (name: string) =>
    formData.getAll(name).map((v) => String(v).trim());
  const egeszNaposFlags = all("egeszNapos");
  const startDates = all("startDate");
  const startTimes = all("startTime");
  const endDates = all("endDate");
  const endTimes = all("endTime");

  const idopontok: IdopontInputs = [];
  for (let i = 0; i < startDates.length; i++) {
    const n = i + 1;
    const startDate = startDates[i];
    const startTime = startTimes[i] ?? "";
    let endDate = endDates[i] ?? "";
    const endTime = endTimes[i] ?? "";
    if (egeszNaposFlags[i] === "1") {
      // Whole days: only the first and (optionally) the last day count.
      if (startDate === "") {
        return { error: `A(z) ${n}. időpontnál add meg az első napot.` };
      }
      if (endDate !== "" && endDate < startDate) {
        return {
          error: `A(z) ${n}. időpont utolsó napja nem lehet az első előtt.`,
        };
      }
      idopontok.push({ start: startDate, end: endDate, egeszNapos: true });
      continue;
    }
    if (startDate === "" || startTime === "") {
      return {
        error: `A(z) ${n}. időpontnál add meg a kezdés napját és idejét.`,
      };
    }
    if (endDate !== "" && endTime === "") {
      return { error: `A(z) ${n}. időpontnál add meg a befejezés idejét is.` };
    }
    if (endTime !== "" && endDate === "") {
      endDate = startDate;
    }
    const start = `${startDate}T${startTime}`;
    const end = endTime === "" ? "" : `${endDate}T${endTime}`;
    // Same fixed format on both sides, so comparing the text works.
    if (end !== "" && end <= start) {
      return {
        error: `A(z) ${n}. időpont befejezése a kezdése után legyen.`,
      };
    }
    idopontok.push({ start, end, egeszNapos: false });
  }
  return { idopontok };
}

/** The message for a time that the DST change skips (Go answers 422). */
export const nonexistentTimeError =
  "Az egyik időpont nem létezik az óraátállítás miatt (pl. március végén 02:00 és 03:00 között). Válassz másik időt.";
