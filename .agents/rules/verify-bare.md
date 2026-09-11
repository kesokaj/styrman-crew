# Run the verification bare, read its status, then commit (BUG-033, BUG-041)

Twice now a commit went through with a red test because the test run sat in
a pipeline inside the `&&` chain that led to `git commit`:

```bash
go test ./... 2>&1 | tail -1 && git commit -q -m "…"   # guards tail's status, not go test's
```

The pipeline's exit status is the filter's. `FAIL` was printed and scrolled
past; the commit landed; the next commit had to undo it.

- Run the verification as its own statement. Capture its status: either let
  the harness show the exit code (`go test ./...` bare) or `cmd > log; rc=$?`
  and print `rc`.
- Read the status BEFORE the commit. Filter the captured output afterwards
  if you need a summary — never in the same chain as the commit.
- Issue the commit as its own tool call once the result has been read.
  Convenience chaining is where this fails; one extra call is the price.
- `set -o pipefail` in a one-off shell is not a substitute: the harness's
  Bash calls do not carry it, and the habit must not depend on it.
- The `verifier` agent follows the same rule: its logs cite the bare
  command and its exit status per step.

Same family as BUG-034 ("silence from your own instrumentation") and
BUG-040 (a green run against the wrong server): a green signal that
measured something else.
