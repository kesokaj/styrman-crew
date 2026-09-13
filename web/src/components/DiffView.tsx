import { reveal } from "../lib/diffReveal";

// reveal lives in ../lib/diffReveal (unit-tested there); re-exported so the
// two diff homes keep one import.
export { reveal };

/**
 * Colourised unified-diff renderer.
 *
 * Extracted from SidePanelGit in v1.22.0 when diffs gained a second home (the
 * main pane, `DiffPane.tsx`). Shared rather than copied on purpose: two
 * renderers would drift, and "the side panel colours hunks differently from
 * the main pane" is the kind of bug nobody files and everybody notices.
 */
export function DiffView({ text }: { text: string }) {
  return (
    <div className="min-h-0 flex-1 overflow-auto bg-bg font-mono text-[11px] leading-snug">
      {text.split("\n").map((line, i) => {
        let cls = "text-text";
        if (line.startsWith("+") && !line.startsWith("+++")) cls = "text-ok";
        else if (line.startsWith("-") && !line.startsWith("---"))
          cls = "text-danger";
        else if (line.startsWith("@@")) cls = "text-accent";
        else if (
          line.startsWith("diff ") ||
          line.startsWith("index ") ||
          line.startsWith("+++") ||
          line.startsWith("---")
        )
          cls = "text-text-dim";
        return (
          <div key={i} className={`whitespace-pre px-2 ${cls}`}>
            {reveal(line) || " "}
          </div>
        );
      })}
    </div>
  );
}
