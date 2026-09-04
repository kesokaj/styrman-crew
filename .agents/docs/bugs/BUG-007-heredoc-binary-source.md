---
owner: bugs
date: 2026-09-04
id: BUG-007
status: fixed
area: web/src/components, web/src/lib (release process, heredoc writes)
---
# BUG-007: heredoc writes put literal control bytes into source, making it silently binary to every reviewer

## Symptom
`web/src/components/DiffPane.tsx` (cache-key separator, offsets 1330/1351/1368)
and `web/src/lib/diffReveal.test.ts` (`\x00`, `\x07`, `\x1b`, `\x7f` test cases)
shipped in v1.22.0 containing literal control bytes instead of escape
sequences. `git diff --numstat` reported `-  -` (binary) for both. Code
worked — NUL is a legal JS string char — so all tests passed; nothing failed.
Every reviewer (phase-5 code review, phase-5 security review, phase-6
reviews) saw "Binary files differ" and signed off without reading the
content. Caught only when the release-range security review inspected file
bytes directly instead of via diff.
`.agents/docs/reviews/2026-09-04-security-release-v1.22.0.md` finding 2.

## Repro
`python3 - <<'PY' ... f.write("...\x00...") ... PY` against a `.ts`/`.tsx`
target: the Python string literal decodes `\x00` to the actual NUL byte
before writing, so the file on disk contains the byte, not the two
characters `\`+`x00`. `git diff --numstat <path>` on such a file reports
`-	-	<path>`.

Recurred three more times same session:
- `internal/httpapi/git_verbs_test.go` — same heredoc mistake, but Go
  rejected it at compile time ("illegal character NUL") and it was fixed
  immediately — loud failure.
- The security agent's own findings file arrived containing the raw
  control bytes it was describing, making the report itself binary.
- A `git commit -F` message with a stray control character was rejected by
  the tool harness.

## Root cause
Heredoc/generator scripts that build source text in a language (Python)
where escape-sequence literals decode to raw bytes, then pipe that decoded
output into a target file in a language (JS/TS, Markdown) that accepts raw
control bytes as valid content with no compile-time signal. The mistake is
invisible in the language that matters because nothing rejects it — only
`git diff`'s binary heuristic notices, and only if someone looks at
`--numstat` instead of the pretty diff.

## Fix
Rewrote the two files' escape sequences as literal `\uXXXX`/`\x` character
sequences in the source (text, not bytes) so `git diff --numstat` reports
normal line counts. Not yet fixed: no automated check exists to catch a
future recurrence — this record is the only guard for now.

## Prevention
- After writing any file via heredoc/generator, verify it is TEXT before
  committing: `git diff --numstat <path>` reporting `-  -`, or `file <path>`
  saying "data", is a stop sign — the content was invisible to every
  reviewer, same class as BUG-005's nonzero-deletion-on-an-"add".
- Never emit a raw control byte into source. Write the escape sequence as
  characters (backslash + `x00`) and let the target language's parser
  decode it at its own compile/parse time. In Python heredocs, prefer raw
  strings (`r"""..."""`) or double the backslashes so Python does not
  decode what should pass through literally.
- A language that accepts the raw byte silently (TypeScript, Markdown) is
  more dangerous here than one that rejects it (Go) — no compiler error to
  catch it, so the binary-diff check above is the only net.
- Applies to review/report artifacts too: a findings file that embeds the
  control bytes it describes defeats its own purpose and must be rewritten
  as escaped text.
- A reviewer seeing "Binary files differ" on a path that should be source
  must refuse to sign off rather than skip the file.

## Related
BUG-005 (write without checking existence, silently deletes tests),
BUG-006 (recursive delete without checking, silently deletes tracked
files) — same family: a file operation whose damage is invisible in the
normal review path (diff, test run, or `rm` output all stay green/clean).
Three related records now share a "verify the file op before trusting it"
shape; recommend this graduate into
`.agents/rules/writing-files.md` as a third section (binary/control-byte
check alongside existence-check and deletion-count check), or a `verifier`
step that runs `git diff --numstat` over the changeset and flags any `-  -`
row on a path that isn't already binary in git's attributes.
`.agents/docs/reviews/2026-09-04-security-release-v1.22.0.md` finding 2.
