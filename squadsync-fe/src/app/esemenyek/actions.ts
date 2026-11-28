"use server";

import { revalidatePath } from "next/cache";
import { redirect, unstable_rethrow } from "next/navigation";
import {
  addIdopont,
  BackendError,
  cancelEsemeny,
  confirmIdopont,
  copyEsemeny,
  type EsemenyInput,
  editEsemeny,
  proposeEsemeny,
  removeIdopont,
  reopenEgyeztetes,
  setHatarido,
  setVisszajelzes,
  type Valasz,
} from "@/lib/backend";
import { requireFelhasznalo } from "@/lib/felhasznalo";
import { idopontokFromForm, nonexistentTimeError } from "./idopont-input";
import { MAX_IDOPONTOK } from "./limits";

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

  const parsed = idopontokFromForm(formData);
  if ("error" in parsed) {
    return parsed;
  }
  input.idopontok = parsed.idopontok;
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
        return { error: nonexistentTimeError };
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
 * Why a change to an Esemény was refused, shown on the Esemény page as
 * ?hiba=<value>: "szervezo" (only the Szervező may), "allapot" (the
 * Esemény's state changed meanwhile), or one of Go's specific reasons.
 */
export type AllapotHiba =
  | "szervezo"
  | "allapot"
  | "nincs_jog"
  | "not_egyeztetes"
  | "idopont_has_answers"
  | "last_idopont";

/** The notice for a refusal (403 or 409) of Go, or null for another error. */
function hibaOf(err: BackendError): AllapotHiba | null {
  switch (err.code) {
    case "not_permitted":
      return "nincs_jog";
    case "not_egyeztetes":
    case "idopont_has_answers":
    case "last_idopont":
      return err.code;
  }
  if (err.status === 403) {
    return "szervezo";
  }
  if (err.status === 409) {
    return "allapot";
  }
  return null;
}

/**
 * Runs one change to an Esemény, then reloads the page without any earlier
 * notice. A refusal (see hibaOf) reloads it with a notice saying why; a lost
 * Tag goes back to the list.
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
      const hiba = hibaOf(err);
      if (hiba !== null) {
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

/** Removes an Időpont during Egyeztetés (see removeIdopont for who may). */
export async function removeIdopontAction(
  esemenyId: string,
  idopontId: string,
) {
  await changeAllapot(esemenyId, (felhasznaloId) =>
    removeIdopont(felhasznaloId, esemenyId, idopontId),
  );
}

/**
 * Any Tag copies the Esemény a week later and lands on the new one. Go
 * announces it like a newly proposed Esemény.
 */
export async function copyEsemenyAction(esemenyId: string) {
  const felhasznalo = await requireFelhasznalo();
  let newId: string;
  try {
    newId = (await copyEsemeny(felhasznalo.id, esemenyId)).id;
  } catch (err) {
    unstable_rethrow(err);
    if (err instanceof BackendError && err.status === 404) {
      redirect("/csoportok");
    }
    throw err;
  }
  redirect(`/esemenyek/${newId}`);
}

export type EditEsemenyState = { error?: string; saved?: boolean };

/** The Szervező edits the title, description and Helyszín. */
export async function editEsemenyAction(
  esemenyId: string,
  _previous: EditEsemenyState,
  formData: FormData,
): Promise<EditEsemenyState> {
  const felhasznalo = await requireFelhasznalo();
  const text = (name: string) => String(formData.get(name) ?? "").trim();
  const title = text("title");
  const helyszin = { name: text("helyszinName"), url: text("helyszinUrl") };
  if (title === "") {
    return { error: "Adj címet az eseménynek." };
  }
  if (helyszin.url !== "" && !isHttpUrl(helyszin.url)) {
    return { error: "A térkép link teljes webcím legyen (https://…)." };
  }
  try {
    await editEsemeny(felhasznalo.id, esemenyId, {
      title,
      description: text("description"),
      helyszin,
    });
  } catch (err) {
    unstable_rethrow(err);
    if (err instanceof BackendError) {
      if (err.status === 404) {
        redirect("/csoportok");
      }
      revalidatePath(`/esemenyek/${esemenyId}`);
      if (err.status === 403) {
        return { error: "Ezt csak az esemény szervezője teheti meg." };
      }
      if (err.code === "esemeny_elmaradt") {
        return { error: "Elmaradt esemény már nem szerkeszthető." };
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
  revalidatePath(`/esemenyek/${esemenyId}`);
  return { saved: true };
}

export type AddIdopontState = { error?: string; added?: boolean };

/** Any Tag adds one Időpont during Egyeztetés. */
export async function addIdopontAction(
  esemenyId: string,
  _previous: AddIdopontState,
  formData: FormData,
): Promise<AddIdopontState> {
  const felhasznalo = await requireFelhasznalo();
  const parsed = idopontokFromForm(formData);
  if ("error" in parsed) {
    return parsed;
  }
  if (parsed.idopontok.length !== 1) {
    return { error: "Adj meg egy időpontot." };
  }
  try {
    await addIdopont(felhasznalo.id, esemenyId, parsed.idopontok[0]);
  } catch (err) {
    unstable_rethrow(err);
    if (err instanceof BackendError) {
      if (err.status === 404) {
        redirect("/csoportok");
      }
      if (err.status === 422) {
        return { error: nonexistentTimeError };
      }
      if (err.status === 400) {
        return { error: "Ez az időpont hibás. Ellenőrizd az adatokat." };
      }
      if (err.status === 409) {
        revalidatePath(`/esemenyek/${esemenyId}`);
        return {
          error:
            err.code === "too_many_idopontok"
              ? `Egy eseménynek legfeljebb ${MAX_IDOPONTOK} időpontja lehet.`
              : "Időpontot csak egyeztetés alatt lehet hozzáadni.",
        };
      }
    }
    throw err;
  }
  revalidatePath(`/esemenyek/${esemenyId}`);
  return { added: true };
}
