# Server-side code formatting with real CLI formatters (v1.7.3)

Date: 2026-08-29. Follow-up to v1.7.2: genuine reformat (not just highlighting)
by running each language's real formatter in the project VM.

## Backend

`POST /api/projects/{project}/format` (`internal/httpapi/format.go`): picks a
formatter by file extension and runs it INSIDE the guest VM, where the
toolchain lives. The editor content round-trips through a guest temp file —
base64 in (so user text is never shell-interpolated), format in place, read
back — with a marker line separating success from the formatter's own error
output (docker exec merges stdout+stderr).

Formatters wired (the tool may be absent → "command not found" is surfaced):
- Go → `gofmt`; Terraform/HCL → `terraform fmt` (falls back to `tofu fmt`);
  Rust → `rustfmt`; Python → `black` (falls back to `ruff format`);
  Shell → `shfmt`; JS/TS/JSON/CSS/HTML/Markdown/YAML/Vue/GraphQL → `prettier`.

## Frontend

`CodeEditor` is now a `forwardRef` exposing `setContent()` (dispatches a
doc-replace transaction, so Format updates the VIEW and flows back to state —
the old JSON Format only changed state, not the editor). The editor Format
button now applies to every supported type: JSON formats locally (instant,
offline); everything else calls the endpoint and replaces the content on
success, or toasts the formatter's error. Files:
`web/src/components/CodeEditor.tsx`, `web/src/components/FileExplorer.tsx`,
`web/src/lib/api.ts`.

## Verification (live, styrman-server 1.7.3)

Against the live "test" project: `gofmt` reflowed a mangled `main.go`; the
Terraform path reformatted `variable "x" {default=1}` via the `tofu fmt`
fallback (terraform not installed); a `.ts` format returned a clean 422
("prettier: command not found"). Only the formatters a project has installed
work — the tool set in a VM is the user's to provision.

## Release

Built + deployed live as styrman-server 1.7.3.
