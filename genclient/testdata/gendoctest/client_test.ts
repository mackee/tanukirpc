import { newClient } from "./client";

const customFetcher = (
  _input: string,
  _init: {
    method: string;
    headers: Record<string, string>;
    body: string | undefined;
  },
) => Promise.resolve(new Response("{}"));

newClient("", customFetcher);

const main = async () => {
  const requests: RequestInit[] = [];
  globalThis.fetch = async (_input, init) => {
    requests.push(init ?? {});
    return new Response("{}");
  };

  const client = newClient();
  await client.get("/ping", {});
  await client.get("/echo", { data: { message: "hello" } });

  if (Object.prototype.hasOwnProperty.call(requests[0], "body")) {
    throw new Error("bodyless request included a body property");
  }
  if (requests[1].body !== '{"message":"hello"}') {
    throw new Error("request body was not sent");
  }
};

void main();
