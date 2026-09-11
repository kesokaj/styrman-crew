import { describe, expect, it } from "vitest";
import { CDP_CLOSED, CdpClient, CdpError, type CdpTransport } from "./cdp";

// A scripted transport: records what the client sent, lets the test inject
// frames and a close, exactly what the relay would carry.
function fakeTransport() {
  const sent: string[] = [];
  const t: CdpTransport & {
    sent: string[];
    receive: (frame: unknown) => void;
    closeFromServer: (code: number, reason: string) => void;
    closed: boolean;
  } = {
    sent,
    closed: false,
    onmessage: null,
    onclose: null,
    send: (d) => {
      sent.push(d);
    },
    close: () => {
      t.closed = true;
    },
    receive: (frame) => t.onmessage?.({ data: frame }),
    closeFromServer: (code, reason) => t.onclose?.({ code, reason }),
  };
  return t;
}

describe("CdpClient", () => {
  it("correlates replies by id and returns the result", async () => {
    const t = fakeTransport();
    const c = new CdpClient(t);
    const p1 = c.send<{ targetInfos: unknown[] }>("Target.getTargets");
    const p2 = c.send("Page.navigate", { url: "http://localhost:3000/" }, "S1");
    expect(JSON.parse(t.sent[0] ?? "")).toEqual({ id: 1, method: "Target.getTargets", params: {} });
    expect(JSON.parse(t.sent[1] ?? "")).toEqual({
      id: 2, method: "Page.navigate", params: { url: "http://localhost:3000/" }, sessionId: "S1",
    });
    // Out of order, as Chromium may answer.
    t.receive(JSON.stringify({ id: 2, result: { frameId: "F" } }));
    t.receive(JSON.stringify({ id: 1, result: { targetInfos: [] } }));
    await expect(p2).resolves.toEqual({ frameId: "F" });
    await expect(p1).resolves.toEqual({ targetInfos: [] });
  });

  it("rejects a protocol error with its code and message", async () => {
    const t = fakeTransport();
    const c = new CdpClient(t);
    const p = c.send("Nope.method");
    t.receive(JSON.stringify({ id: 1, error: { code: -32601, message: "'Nope.method' wasn't found" } }));
    await expect(p).rejects.toMatchObject({ name: "CdpError", code: -32601 });
  });

  it("fans events out with their sessionId and honours unsubscribe", () => {
    const t = fakeTransport();
    const c = new CdpClient(t);
    const seen: Array<[unknown, string | undefined]> = [];
    const off = c.on("Page.screencastFrame", (params, sid) => seen.push([params, sid]));
    t.receive(JSON.stringify({ method: "Page.screencastFrame", params: { data: "x" }, sessionId: "S1" }));
    t.receive(JSON.stringify({ method: "Other.event", params: {} }));
    off();
    t.receive(JSON.stringify({ method: "Page.screencastFrame", params: { data: "y" } }));
    expect(seen).toEqual([[{ data: "x" }, "S1"]]);
  });

  it("drops malformed frames, non-text frames and replies nobody waits for", async () => {
    const t = fakeTransport();
    const c = new CdpClient(t);
    t.receive("not json");
    t.receive(new ArrayBuffer(4));
    t.receive(JSON.stringify({ id: 42, result: {} }));
    t.receive(JSON.stringify(null));
    // Still usable afterwards.
    const p = c.send("Browser.getVersion");
    t.receive(JSON.stringify({ id: 1, result: { product: "Chrome/152" } }));
    await expect(p).resolves.toEqual({ product: "Chrome/152" });
  });

  it("rejects every pending call when the socket closes, then refuses new ones", async () => {
    const t = fakeTransport();
    const c = new CdpClient(t);
    let closed: [number, string] | undefined;
    c.onClose = (code, reason) => (closed = [code, reason]);
    const p = c.send("Target.getTargets");
    t.closeFromServer(1011, "browser disconnected");
    await expect(p).rejects.toMatchObject({ code: CDP_CLOSED });
    expect(closed).toEqual([1011, "browser disconnected"]);
    expect(c.closed).toBe(true);
    await expect(c.send("Target.getTargets")).rejects.toBeInstanceOf(CdpError);
    // A late reply after close is ignored, not a crash.
    t.receive(JSON.stringify({ id: 1, result: {} }));
  });

  it("close() closes the transport and reports once", () => {
    const t = fakeTransport();
    const c = new CdpClient(t);
    let n = 0;
    c.onClose = () => n++;
    c.close();
    c.close();
    t.closeFromServer(1000, "");
    expect(t.closed).toBe(true);
    expect(n).toBe(1);
  });
});
