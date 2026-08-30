import { useRef, useState } from "react";
import { api, type HistoryMatch } from "../lib/api";
import { usePolled } from "../lib/hooks";
import { toast } from "../lib/toast";

export function HistoryTab({ project }: { project: string }) {
  const { data } = usePolled(() => api.listHistory(project), 10000, [project]);
  const [openId, setOpenId] = useState<string | null>(null);
  const [text, setText] = useState("");
  const [query, setQuery] = useState("");
  const [matches, setMatches] = useState<HistoryMatch[] | null>(null);
  const [searching, setSearching] = useState(false);
  // Two quick Enters can resolve out of order — only the newest wins.
  const searchGen = useRef(0);
  const transcripts = data?.transcripts ?? [];
  const titleOf = (id: string) =>
    transcripts.find((t) => t.id === id)?.title || id;

  const openGen = useRef(0);
  const open = async (id: string) => {
    const gen = ++openGen.current; // two quick opens: only the last one wins
    setOpenId(id);
    setText("loading…");
    const r = await api
      .readHistory(project, id)
      .catch((e: Error) => ({ text: e.message }));
    if (gen === openGen.current) setText(r.text);
  };

  const runSearch = async (q: string) => {
    const trimmed = q.trim();
    if (!trimmed) {
      setMatches(null);
      return;
    }
    const gen = ++searchGen.current;
    setSearching(true);
    try {
      const r = await api.searchHistory(project, trimmed);
      if (gen !== searchGen.current) return; // stale response
      setMatches(r.matches);
    } catch (e) {
      if (gen === searchGen.current) toast("error", (e as Error).message);
    } finally {
      if (gen === searchGen.current) setSearching(false);
    }
  };

  if (openId) {
    return (
      <div className="flex h-full flex-col">
        <div className="flex items-center gap-2 border-b border-border px-2 py-1.5 text-xs">
          <button
            onClick={() => setOpenId(null)}
            className="text-text-dim hover:text-text"
          >
            ← back
          </button>
          <span className="truncate font-medium">{titleOf(openId)}</span>
          <span className="shrink-0 font-mono text-[10px] text-text-dim">
            {openId}
          </span>
        </div>
        <pre className="min-h-0 flex-1 overflow-auto whitespace-pre-wrap p-2 font-mono text-[11px] leading-snug">
          {text}
        </pre>
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col">
      <div className="border-b border-border p-2">
        <input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && runSearch(query)}
          placeholder="Search all transcripts (Enter)"
          className="w-full rounded-lg border border-border bg-bg px-2.5 py-1.5 text-xs outline-none focus:border-accent"
          aria-label="Search history"
        />
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto p-2">
        {matches !== null ? (
          <>
            <div className="flex items-center px-1 pb-1">
              <span className="flex-1 text-[11px] text-text-dim">
                {searching
                  ? "Searching…"
                  : `${matches.length} match${matches.length === 1 ? "" : "es"}`}
              </span>
              <button
                onClick={() => {
                  setMatches(null);
                  setQuery("");
                }}
                className="text-[11px] text-text-dim hover:text-text"
              >
                clear
              </button>
            </div>
            {matches.map((m, i) => (
              <button
                key={`${m.transcript}-${i}`}
                onClick={() => open(m.transcript)}
                className="mb-0.5 flex w-full flex-col gap-0.5 rounded-lg px-1.5 py-1 text-left hover:bg-bg-hover"
              >
                <span className="text-[10px] text-accent">
                  {m.title || m.transcript}
                </span>
                <span className="break-all font-mono text-[11px] leading-snug">
                  {m.line}
                </span>
              </button>
            ))}
          </>
        ) : (
          <>
            <p className="px-1 pb-2 text-[11px] text-text-dim">
              Everything typed and printed in this project's terminals — you and
              the agents — captured to the shared home, surviving container
              rebuilds.
            </p>
            {transcripts.length === 0 && (
              <p className="px-1 text-xs text-text-dim">No transcripts yet.</p>
            )}
            {transcripts.map((t) => (
              <button
                key={t.id}
                onClick={() => open(t.id)}
                className="flex w-full items-center gap-2 rounded-lg px-1.5 py-1 text-left text-xs hover:bg-bg-hover"
              >
                <span className="min-w-0 flex-1 truncate">
                  {t.title || t.id}
                </span>
                {t.title && (
                  <span className="shrink-0 font-mono text-[10px] text-text-dim">
                    {t.id}
                  </span>
                )}
                <span className="shrink-0 text-[10px] text-text-dim">
                  {(t.size / 1024).toFixed(0)} KB ·{" "}
                  {t.modified.slice(5, 16).replace("T", " ")}
                </span>
              </button>
            ))}
          </>
        )}
      </div>
    </div>
  );
}
