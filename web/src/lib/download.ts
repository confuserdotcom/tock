// Downloads from the API: fetched with the token like any call, then saved
// through a temporary link, since a plain link can't carry the header.
import { getRaw } from "./api";

/** fileName is the name in a Content-Disposition header, without any
 *  directory part, or def. */
export function fileName(disposition: string | null, def: string): string {
  const m = disposition?.match(/filename="?([^";]+)"?/);
  const name = m?.[1].split(/[/\\]/).pop();
  return name || def;
}

/** download fetches path and saves it under the name the server gives. */
export async function download(path: string, def: string): Promise<string> {
  const resp = await getRaw(path);
  const name = fileName(resp.headers.get("Content-Disposition"), def);
  const url = URL.createObjectURL(await resp.blob());
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 60_000);
  return name;
}
