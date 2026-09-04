/**
 * Characters that can make a diff READ differently from how it applies:
 * the bidi overrides/isolates behind "Trojan Source", the invisible LTR/RTL
 * marks, and C0/C7 control bytes. React escapes markup, so this is not about
 * XSS — it is that this pane's whole job is reviewing a change BEFORE you
 * commit it, and a reviewer must not be shown a reordered line (security
 * review 2026-09-04). Tab is kept: it is real, meaningful diff content.
 */
// eslint-disable-next-line no-control-regex
const INVISIBLE = /[\u0000-\u0008\u000B-\u001F\u007F-\u009F\u061C\u200E\u200F\u202A-\u202E\u2066-\u2069]/g;

/** Render them VISIBLE rather than dropping them: a silently removed byte is
 *  its own kind of lie about what the file contains. */
export const reveal = (line: string) =>
  line.replace(INVISIBLE, (c) => `<U+${c.codePointAt(0)!.toString(16).toUpperCase().padStart(4, "0")}>`);

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
