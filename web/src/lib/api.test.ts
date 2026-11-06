// get, post and getRaw share one request: the token header, no cache, and
// the server's {"error"} as the ApiError message.
import { test, expect, vi, afterEach } from "vitest";

const seen: { path: string; init: RequestInit }[] = [];

function serve(status: number, body: string) {
  seen.length = 0;
  vi.stubGlobal("fetch", async (path: string, init: RequestInit) => {
    seen.push({ path, init });
    return new Response(body, { status, statusText: status === 200 ? "OK" : "Teapot" });
  });
}

async function api() {
  vi.resetModules();
  const store = new Map<string, string>([["tock-token", "a".repeat(64)]]);
  vi.stubGlobal("sessionStorage", { getItem: (k: string) => store.get(k) ?? null, setItem: () => {} });
  vi.stubGlobal("location", { hash: "", pathname: "/", search: "" });
  const m = await import("./api");
  m.readToken();
  return m;
}

afterEach(() => vi.unstubAllGlobals());

test("every request carries the token and skips the cache", async () => {
  const { get, post, getRaw } = await api();
  serve(200, '{"x":1}');
  expect(await get("/api/a")).toEqual({ x: 1 });
  await post("/api/b", { y: 2 });
  const r = await getRaw("/api/c");
  expect(await r.text()).toBe('{"x":1}');
  for (const s of seen) {
    expect((s.init.headers as Record<string, string>)["X-Tock-Token"]).toBe("a".repeat(64));
    expect(s.init.cache).toBe("no-store");
  }
  expect(seen[0].init.method).toBeUndefined();
  expect(seen[1].init.method).toBe("POST");
  expect(seen[1].init.body).toBe('{"y":2}');
});

test("errors read the same way for JSON calls and downloads", async () => {
  const { get, getRaw, ApiError } = await api();
  for (const [body, msg] of [
    ['{"error":"from is after to"}', "from is after to"],
    ["plain text", "plain text"],
    ["", "Teapot"],
  ]) {
    serve(400, body);
    for (const f of [() => get("/x"), () => getRaw("/x")]) {
      const err = await f().catch((e) => e);
      expect(err).toBeInstanceOf(ApiError);
      expect(err).toMatchObject({ status: 400, message: msg });
    }
  }
});
