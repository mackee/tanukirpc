import { strict as assert } from "node:assert";
import { test } from "node:test";
import { newClient } from "../../gendoctest/client";

test("custom fetcher signature is accepted", () => {
  const customFetcher = (
    _input: string,
    _init: {
      method: string;
      headers: Record<string, string>;
      body: string | undefined;
    },
  ) => Promise.resolve(new Response("{}"));

  newClient("", customFetcher);
});

test("default fetcher omits body on bodyless requests", async (t) => {
  const requests: RequestInit[] = [];
  t.mock.method(globalThis, "fetch", async (_input: RequestInfo | URL, init?: RequestInit) => {
    requests.push(init ?? {});
    return new Response("{}");
  });

  await newClient().get("/ping", {});

  assert.equal(requests.length, 1);
  assert.equal(Object.hasOwn(requests[0]!, "body"), false);
});

test("default fetcher sends JSON-encoded request body", async (t) => {
  const requests: RequestInit[] = [];
  t.mock.method(globalThis, "fetch", async (_input: RequestInfo | URL, init?: RequestInit) => {
    requests.push(init ?? {});
    return new Response("{}");
  });

  await newClient().get("/echo", { data: { message: "hello" } });

  assert.equal(requests.length, 1);
  assert.equal(requests[0]!.body, '{"message":"hello"}');
});
