"use server";

import { revalidatePath } from "next/cache";
import { redirect, unstable_rethrow } from "next/navigation";
import {
  BackendError,
  cancelEsemeny,
  confirmIdopont,
  type EsemenyInput,
  proposeEsemeny,
  reopenEgyeztetes,
  setHatarido,
  setVisszajelzes,
  type Valasz,
} from "@/lib/backend";
import { requireFelhasznalo } from "@/lib/felhasznalo";

export type ProposeEsemenyState = { error?: string };

/**
 * Proposes an Esemény in a Csoport. The form sends each Időpont as a start
 * date and time and an optional end date and time, all Budapest local time;
 * Go converts them to UTC. The checks here only give friendly Hungarian
 * messages: Go has the final say.
 */
export async function proposeEsemenyAction(
  csoportId: string,
  _previous: ProposeEsemenyState,
  formData: FormData,
): Promise<ProposeEsemenyState> {
  const felhasznalo = await requireFelhasznalo();
  const text = (name: string) => String(formData.get(name) ?? "").trim();
  const all = (name: string) =>
    formData.getAll(name).map((v) => String(v).trim());

  const input: EsemenyInput = {
    title: text("title"),
    description: text("description"),
    helyszin: { name: text("helyszinName"), url: text("helyszinUrl") },
    idopontok: [],
    hatarido: "",
  };
  const hatarido = hataridoFrom(formData);
  if ("error" in hatarido) {
    return hatarido;
  }
  input.hatarido = hatarido.value;
  if (input.title === "") {
    return { error: "Adj címet az eseménynek." };
  }
  const url = input.helyszin.url;
  if (url !== "" && !isHttpUrl(url)) {
    return {
      error: "A térkép link teljes webcím legyen (https://…).",
    };
  }

  const egeszNaposFlags = all("egeszNapos");
  const startDates = all("startDate");
  const startTimes = all("startTime");
  const endDates = all("endDate");
  const endTimes = all("endTime");
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
      input.idopontok.push({
        start: startDate,
        end: endDate,
        egeszNapos: true,
      });
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
    input.idopontok.push({ start, end, egeszNapos: false });
  }
  if (input.idopontok.length === 0) {
    return { error: "Adj meg legalább egy időpontot." };
  }

  let esemenyId: string;
  try {
    esemenyId = (await proposeEsemeny(felhasznalo.id, csoportId, input)).id;
  } catch (err) {
    unstable_rethrow(err);
    if (err instanceof BackendError) {
      if (err.status === 404) {
        redirect("/csoportok");
      }
      if (err.status === 422) {
        return {
          error:
            "Az egyik időpont nem létezik az óraátállítás miatt (pl. március végén 02:00 és 03:00 között). Válassz másik időt.",
        };
      }
      if (err.status === 400 && err.code === "hatarido_not_future") {
        return { error: hataridoNotFuture };
      }
      if (err.status === 400) {
        return {
          error:
            "Valamelyik mező hibás vagy túl hosszú. Ellenőrizd az adatokat.",
        };
      }
    }
    throw err;
  }
  redirect(`/esemenyek/${esemenyId}`);
}

const hataridoNotFuture = "A szavazási határidő a jövőben legyen.";

/**
 * The optional voting Határidő from the form's hataridoDate and
 * hataridoTime fields (Budapest local time): "" when both are empty.
 */
function hataridoFrom(
  formData: FormData,
): { value: string } | { error: string } {
  const date = String(formData.get("hataridoDate") ?? "").trim();
  const time = String(formData.get("hataridoTime") ?? "").trim();
  if (date === "" && time === "") {
    return { value: "" };
  }
  if (date === "" || time === "") {
    return { error: "A határidőhöz add meg a napot és az időt is." };
  }
  return { value: `${date}T${time}` };
}

export type HataridoState = { error?: string };

/**
 * The Szervező sets or changes the voting Határidő, or clears it when the
 * form was sent with intent=clear.
 */
export async function setHataridoAction(
  esemenyId: string,
  _previous: HataridoState,
  formData: FormData,
): Promise<HataridoState> {
  const felhasznalo = await requireFelhasznalo();
  let value = "";
  if (formData.get("intent") !== "clear") {
    const hatarido = hataridoFrom(formData);
    if ("error" in hatarido) {
      return hatarido;
    }
    if (hatarido.value === "") {
      return { error: "Add meg a határidő napját és idejét." };
    }
    value = hatarido.value;
  }
  try {
    await setHatarido(felhasznalo.id, esemenyId, value);
  } catch (err) {
    unstable_rethrow(err);
    if (err instanceof BackendError) {
      if (err.status === 404) {
        redirect("/csoportok");
      }
      if (err.status === 422) {
        return {
          error:
            "Ez az időpont nem létezik az óraátállítás miatt. Válassz másik időt.",
        };
      }
      if (err.status === 400) {
        return {
          error:
            err.code === "hatarido_not_future"
              ? hataridoNotFuture
              : "Ez az időpont nem létezik vagy hibás.",
        };
      }
      if (err.status === 403 || err.status === 409) {
        revalidatePath(`/esemenyek/${esemenyId}`);
        return {
          error:
            err.status === 403
              ? "Ezt csak az esemény szervezője teheti meg."
              : "Határidőt csak egyeztetés alatt lehet beállítani.",
        };
      }
    }
    throw err;
  }
  revalidatePath(`/esemenyek/${esemenyId}`);
  return {};
}

// Server actions are public endpoints: the bound valasz can't be trusted.
function isValasz(value: unknown): value is Valasz {
  return value === "igen" || value === "talan" || value === "nem";
}

function isHttpUrl(value: string): boolean {
  if (!URL.canParse(value)) {
    return false;
  }
  const url = new URL(value);
  return (
    (url.protocol === "http:" || url.protocol === "https:") && url.host !== ""
  );
}

/** Gives or changes the caller's Visszajelzés on one Időpont. */
export async function setVisszajelzesAction(
  esemenyId: string,
  idopontId: string,
  valasz: Valasz,
) {
  if (!isValasz(valasz)) {
    throw new Error(`unknown valasz ${String(valasz)}`);
  }
  const felhasznalo = await requireFelhasznalo();
  try {
    await setVisszajelzes(felhasznalo.id, esemenyId, idopontId, valasz);
  } catch (err) {
    unstable_rethrow(err);
    if (err instanceof BackendError) {
      if (err.status === 404) {
        // Stale page: no longer a Tag, or the Időpont is gone.
        redirect("/csoportok");
      }
      if (err.status === 409) {
        // The Időpont can't be answered in the Esemény's current state
        // (ErrIdopontNotAnswerable): the refreshed page shows why.
        revalidatePath(`/esemenyek/${esemenyId}`);
        return;
      }
    }
    throw err;
  }
  revalidatePath(`/esemenyek/${esemenyId}`);
}

/**
 * Why a lifecycle change was refused, shown on the Esemény page as
 * ?hiba=<value>.
 */
export type AllapotHiba = "szervezo" | "allapot";

/**
 * Runs one of the Szervező's lifecycle changes, then reloads the page
 * without any earlier notice. A refusal (no longer the Szervező: 403, or
 * the Esemény already changed: 409) reloads it with a notice saying why; a
 * lost Tag goes back to the list.
 */
async function changeAllapot(
  esemenyId: string,
  change: (felhasznaloId: string) => Promise<void>,
) {
  const felhasznalo = await requireFelhasznalo();
  try {
    await change(felhasznalo.id);
  } catch (err) {
    unstable_rethrow(err);
    if (err instanceof BackendError) {
      if (err.status === 404) {
        redirect("/csoportok");
      }
      if (err.status === 403 || err.status === 409) {
        const hiba: AllapotHiba = err.status === 403 ? "szervezo" : "allapot";
        revalidatePath(`/esemenyek/${esemenyId}`);
        redirect(`/esemenyek/${esemenyId}?hiba=${hiba}`);
      }
    }
    throw err;
  }
  revalidatePath(`/esemenyek/${esemenyId}`);
  redirect(`/esemenyek/${esemenyId}`);
}

/** The Szervező confirms one Időpont: the Esemény becomes Véglegesítve. */
export async function confirmIdopontAction(
  esemenyId: string,
  idopontId: string,
) {
  await changeAllapot(esemenyId, (felhasznaloId) =>
    confirmIdopont(felhasznaloId, esemenyId, idopontId),
  );
}

/** The Szervező reopens Egyeztetés, keeping every Visszajelzés. */
export async function reopenEgyeztetesAction(esemenyId: string) {
  await changeAllapot(esemenyId, (felhasznaloId) =>
    reopenEgyeztetes(felhasznaloId, esemenyId),
  );
}

/** The Szervező cancels the Esemény: it becomes Elmaradt, for good. */
export async function cancelEsemenyAction(esemenyId: string) {
  await changeAllapot(esemenyId, (felhasznaloId) =>
    cancelEsemeny(felhasznaloId, esemenyId),
  );
}
