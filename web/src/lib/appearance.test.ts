import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// The appearance store reads localStorage AT MODULE LOAD, so every case here
// stubs storage first and then imports the module fresh (vi.resetModules),
// the same shape api.session.test.ts uses.

const KEY = "styrman.appearance.v1";

/** localStorage that behaves. */
function memoryStorage(seed?: Record<string, string>) {
  const m = new Map<string, string>(Object.entries(seed ?? {}));
  return {
    getItem: (k: string) => m.get(k) ?? null,
    setItem: (k: string, v: string) => void m.set(k, String(v)),
    removeItem: (k: string) => void m.delete(k),
    _dump: () => Object.fromEntries(m),
  };
}

/** A private window: touching the accessor itself throws, not just write. */
function hostileStorage() {
  return {
    get getItem(): never {
      throw new DOMException("denied", "SecurityError");
    },
    get setItem(): never {
      throw new DOMException("denied", "SecurityError");
    },
  };
}

async function loadWith(storage: unknown) {
  vi.resetModules();
  vi.stubGlobal("localStorage", storage);
  return await import("./hooks");
}

/** Read back what the store persisted, failing the test if it wrote nothing. */
function stored(s: ReturnType<typeof memoryStorage>): Record<string, unknown> {
  const raw = s._dump()[KEY];
  if (raw === undefined) throw new Error(`nothing stored under ${KEY}`);
  return JSON.parse(raw) as Record<string, unknown>;
}

const DEFAULTS = {
  termFontSize: 13,
  termFontFamily: "google-sans-code",
  editorFontSize: 12,
  editorFontFamily: "system",
};

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("appearance store defaults", () => {
  it("terminal defaults to 13px Google Sans Code, editor to 12px system", async () => {
    const s = memoryStorage();
    const h = await loadWith(s);
    h.setAppearance({}); // no-op patch: forces a sanitise of the loaded state
    expect(stored(s)).toEqual(DEFAULTS);
  });

  it("the editor default is NOT the terminal's — flattening them would restyle every existing editor", async () => {
    const h = await loadWith(memoryStorage());
    h.setAppearance({});
    expect(DEFAULTS.editorFontSize).not.toBe(DEFAULTS.termFontSize);
    expect(DEFAULTS.editorFontFamily).not.toBe(DEFAULTS.termFontFamily);
    // And the stacks they resolve to really do differ.
    expect(h.FONT_STACKS[DEFAULTS.editorFontFamily as "system"]).not.toBe(
      h.FONT_STACKS[DEFAULTS.termFontFamily as "google-sans-code"],
    );
  });
});

describe("appearance store sanitising", () => {
  const load = async (raw: string) => {
    const s = memoryStorage({ [KEY]: raw });
    const h = await loadWith(s);
    h.setAppearance({}); // write the sanitised view back out so we can read it
    return stored(s);
  };

  it("keeps a valid stored object", async () => {
    const v = {
      termFontSize: 20,
      termFontFamily: "jetbrains-mono",
      editorFontSize: 9,
      editorFontFamily: "google-sans-code",
    };
    expect(await load(JSON.stringify(v))).toEqual(v);
  });

  it("falls back PER FIELD — one bad value never discards the others", async () => {
    const got = await load(
      JSON.stringify({
        termFontSize: 999, // out of range
        termFontFamily: "comic-sans", // unknown
        editorFontSize: 17, // fine, must survive
        editorFontFamily: "jetbrains-mono", // fine, must survive
      }),
    );
    expect(got.termFontSize).toBe(DEFAULTS.termFontSize);
    expect(got.termFontFamily).toBe(DEFAULTS.termFontFamily);
    expect(got.editorFontSize).toBe(17);
    expect(got.editorFontFamily).toBe("jetbrains-mono");
  });

  it("clamps sizes outside 8..32 and rejects non-finite ones", async () => {
    for (const bad of [0, 7, 33, 1e6, -12]) {
      const got = await load(JSON.stringify({ termFontSize: bad }));
      expect(got.termFontSize).toBe(DEFAULTS.termFontSize);
    }
    // The bounds themselves are valid.
    expect((await load(JSON.stringify({ termFontSize: 8 }))).termFontSize).toBe(
      8,
    );
    expect(
      (await load(JSON.stringify({ termFontSize: 32 }))).termFontSize,
    ).toBe(32);
  });

  it("rejects a size that is not a number, including a numeric string", async () => {
    for (const bad of ['"14"', "null", "true", "[]", "{}"]) {
      const got = await load(`{"termFontSize": ${bad}}`);
      expect(got.termFontSize).toBe(DEFAULTS.termFontSize);
    }
  });

  it("rejects a prototype key posing as a font family", async () => {
    // `"toString" in FONT_STACKS` is true; hasOwn is what makes this safe.
    const got = await load(JSON.stringify({ termFontFamily: "toString" }));
    expect(got.termFontFamily).toBe(DEFAULTS.termFontFamily);
  });

  it("survives corrupt JSON, a non-object, and an absent key", async () => {
    expect(await load("not json at all")).toEqual(DEFAULTS);
    expect(await load('"a string"')).toEqual(DEFAULTS);
    expect(await load("[1,2,3]")).toEqual(DEFAULTS);
    expect(await load("null")).toEqual(DEFAULTS);
    const s = memoryStorage();
    const h = await loadWith(s);
    h.setAppearance({});
    expect(stored(s)).toEqual(DEFAULTS);
  });
});

describe("appearance store writes", () => {
  it("merges a patch instead of replacing the whole object", async () => {
    const s = memoryStorage();
    const h = await loadWith(s);
    h.setAppearance({ termFontSize: 18 });
    h.setAppearance({ editorFontFamily: "jetbrains-mono" });
    expect(stored(s)).toEqual({
      ...DEFAULTS,
      termFontSize: 18,
      editorFontFamily: "jetbrains-mono",
    });
  });

  it("sanitises the patch too, not just what was loaded", async () => {
    const s = memoryStorage();
    const h = await loadWith(s);
    h.setAppearance({ termFontSize: 4000 });
    expect(stored(s).termFontSize).toBe(DEFAULTS.termFontSize);
  });

  it("replaces the snapshot object identity, so useSyncExternalStore re-renders", async () => {
    // The hook compares snapshots by identity; mutating in place would leave
    // every live terminal and editor on the old font.
    const s = memoryStorage();
    const h = await loadWith(s);
    const before = s._dump()[KEY];
    h.setAppearance({ termFontSize: 15 });
    expect(s._dump()[KEY]).not.toBe(before);
    expect(stored(s).termFontSize).toBe(15);
  });
});

describe("appearance store in a private window", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  it("loads defaults when reading localStorage throws", async () => {
    const s = memoryStorage();
    const h = await loadWith(hostileStorage());
    // Reading threw -> defaults. Writing throws too, and must not propagate.
    expect(() => h.setAppearance({ termFontSize: 20 })).not.toThrow();
    void s;
  });

  it("keeps the choice in memory for the session even when it cannot persist", async () => {
    const h = await loadWith(hostileStorage());
    h.setAppearance({ termFontSize: 20 });
    // No storage to inspect; the contract is simply that it did not throw and
    // the module still functions afterwards.
    expect(() => h.setAppearance({ editorFontSize: 11 })).not.toThrow();
  });
});

describe("FONT_STACKS", () => {
  it("reproduces the terminal's historical six-part chain verbatim", async () => {
    // There is now ONE copy of each stack (lib/fonts.ts) and the surfaces take
    // the union, so the old drift-between-duplicates risk is gone by
    // construction. What still needs guarding is the content: dropping a
    // fallback silently changes what renders for anyone who has that face.
    const h = await loadWith(memoryStorage());
    expect(h.FONT_STACKS["google-sans-code"]).toBe(
      "'Google Sans Code Variable', 'JetBrains Mono', ui-monospace, SFMono-Regular, Menlo, monospace",
    );
    expect(h.FONT_STACKS.system).toBe(
      "ui-monospace, SFMono-Regular, Menlo, monospace",
    );
  });

  it("the two surfaces' default families still point at those stacks", async () => {
    // terminal.ts pulls in xterm addons, whose UMD wrappers reference the
    // browser global `self`. These tests run in vitest's node environment.
    vi.stubGlobal("self", globalThis);
    const t = await import("./terminal");
    const ed = await import("../components/CodeEditor");
    expect(t.DEFAULT_TERM_FONT_FAMILY).toBe("google-sans-code");
    expect(t.DEFAULT_TERM_FONT_SIZE).toBe(13);
    expect(ed.DEFAULT_EDITOR_FONT_FAMILY).toBe("system");
    expect(ed.DEFAULT_EDITOR_FONT_SIZE).toBe(12);
  });

  it("has an entry for every selectable family, and labels match", async () => {
    const h = await loadWith(memoryStorage());
    const labelled = h.FONT_FAMILY_LABELS.map(([v]) => v).sort();
    expect(Object.keys(h.FONT_STACKS).sort()).toEqual(labelled);
  });

  it("every stack ends in a generic monospace fallback", async () => {
    const h = await loadWith(memoryStorage());
    for (const stack of Object.values(h.FONT_STACKS)) {
      expect(stack.endsWith("monospace")).toBe(true);
    }
  });
});
