# File editor: keep-open on save, resizable window, more languages (v1.7.2)

Date: 2026-08-29. Three operator requests for the file explorer's editor.

## 1. Save no longer closes the window

`FileViewer.save` used to call `onClose()` after writing. Now it keeps the
editor open: it updates a `savedContent` baseline (so `dirty` re-computes and
the Save button re-disables until the next edit) and shows a "Saved" toast.
Closing still prompts on unsaved changes. File: `web/src/components/FileExplorer.tsx`.

## 2. Resizable editor window with a default size

The modal is now a resizable box (native `resize: both` handle, bottom-right)
with a default of `min(56rem, 92vw)` × `80vh`, capped to the viewport
(`max-w-[97vw]`/`max-h-[95vh]`) with sensible minimums. The chosen size is
remembered across opens in `localStorage` (`styrman-editor-size`, via a
`ResizeObserver`; all storage access try/catch-guarded). File:
`web/src/components/FileExplorer.tsx`.

## 3. More editor languages

Added to the filename→grammar map (`web/src/components/CodeEditor.tsx`), each
lazy-loaded on first open:

- **Java** (.java), **C/C++** (.c/.h/.cc/.cpp/.cxx/.hpp/.hh/.hxx), **Rust**
  (.rs), **PHP** (.php), **SQL** (.sql), **XML** (.xml/.svg/.xsd/.xsl/.plist),
  **Terraform/HCL** (.tf/.tfvars/.hcl, via `codemirror-lang-hcl`).
- Already covered and unchanged: TypeScript/JS, Go, Python, JSON, YAML,
  Markdown, HTML, CSS, Shell, Dockerfile, TOML. **Kubernetes manifests and
  docker-compose** are YAML — highlighted via the `.yaml`/`.yml` match.

New deps: `@codemirror/lang-{java,cpp,rust,php,sql,xml}` + `codemirror-lang-hcl`.

### Scope note

This is **syntax highlighting + language-aware indentation**, not
reformatting. Full auto-format (prettier/gofmt-style) and linting are a much
heavier lift (bundled formatters/linters or an LSP) and were left out of this
pass — the existing JSON "Format" button stays. Can revisit per-language format
if wanted.

## Release

Built + deployed live as styrman-server 1.7.2.
