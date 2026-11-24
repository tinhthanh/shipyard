import type { Session } from "@/lib/api";

// POST sessions/{name}/account in ?mock=1 (the fork's; berthd's
// sessionaccount.go): the session's account changes to the one asked for,
// "default" or a folder whose name is the account's.
export function mockAccountCall(list: Session[] | undefined, method: string, path: string, body: unknown, delay: <T>(v: T) => Promise<T>): Promise<unknown> | undefined {
  const m = method === "POST" ? /^sessions\/([^/?]+)\/account$/.exec(path) : null;
  if (!m) return undefined;
  const s = list?.find((x) => x.name === decodeURIComponent(m[1]));
  if (!s) return Promise.reject(new Error("no session with that name"));
  const asked = String((body as { account?: string })?.account ?? "");
  const name = asked === "default" || !asked ? "default" : (asked.split("/").pop() ?? asked);
  if (name === s.account) return Promise.reject(new Error(`${s.name} already uses ${name}`));
  s.account = name;
  return delay({ ...s });
}
