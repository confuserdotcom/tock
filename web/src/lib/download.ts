// Downloads from the API: fetched with the token like any call, then saved
// through a temporary link, since a plain link can't carry the header.
import { getRaw } from "./api";

/** NATIVE is the tock window: there the save goes through a save sheet and
 *  tockbar reports back how it ended. */
const NATIVE = /tock-app/.test(navigator.userAgent);

/** fileName is the name in a Content-Disposition header, without any
 *  directory part, or def. The RFC 5987 filename*= form wins over the
 *  plain one, as browsers do. */
export function fileName(disposition: string | null, def: string): string {
  let raw: string | undefined;
  const ext = disposition?.match(/filename\*\s*=\s*[^']*'[^']*'([^;]+)/i);
  if (ext) {
    try {
      raw = decodeURIComponent(ext[1].trim());
    } catch {
      raw = undefined;
    }
  } else {
    raw = disposition?.match(/filename\s*=\s*"?([^";]+)"?/i)?.[1];
  }
  const name = raw?.split(/[/\\]/).pop()?.trim();
  return name || def;
}

/** Saved is how a download ended: saved under name, or not (cancelled
 *  without an error, failed with one). In a browser the browser takes it
 *  from the click on, so saved there means handed over. */
export type Saved = { saved: boolean; name: string; error?: string; browser?: boolean };

/** The tock window's report on a download, dispatched on window. */
type NativeResult = { ok: boolean; name?: string; error?: string };

/** download fetches path and saves it under the name the server gives. */
export async function download(path: string, def: string): Promise<Saved> {
  const resp = await getRaw(path);
  const name = fileName(resp.headers.get("Content-Disposition"), def);
  return saveBlob(await resp.blob(), name);
}

/** saveBlob saves blob as name: through the save sheet in the tock window,
 *  else as a browser download. */
export async function saveBlob(blob: Blob, name: string): Promise<Saved> {
  const url = URL.createObjectURL(blob);
  // In the tock window the blob stays alive until tockbar says the save
  // ended: the save sheet waits on the user for as long as it takes.
  const done = NATIVE
    ? new Promise<Saved>((resolve) => {
        window.addEventListener(
          "tock-download",
          (ev) => {
            const r = (ev as CustomEvent<NativeResult>).detail;
            resolve({ saved: r.ok, name: r.name || name, error: r.error });
          },
          { once: true },
        );
      })
    : Promise.resolve<Saved>({ saved: true, name, browser: true });
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  document.body.appendChild(a);
  a.click();
  a.remove();
  const res = await done;
  if (NATIVE) URL.revokeObjectURL(url);
  else setTimeout(() => URL.revokeObjectURL(url), 60_000);
  return res;
}
