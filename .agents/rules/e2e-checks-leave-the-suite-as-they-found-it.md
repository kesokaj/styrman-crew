# An e2e check leaves the suite as it found it (BUG-021, BUG-078)

Twice now one failing or state-changing check in `web/e2e/run.mjs` has been
reported as many defects:

- **BUG-021** (2026-09-09): network-toggle checks placed early rebuilt the
  project container, and **17 unrelated checks failed** behind them. The fix
  moved the toggles to the end of the run, where nothing follows them.
- **BUG-078** (2026-09-22): the truncation check navigated the file explorer
  and, when it failed, left it at the wrong path — **8 later checks** that
  open a file from that listing failed too (`passed=84 failed=9` for one
  cause). Found only by deliberately making the check fail as a must-fail
  proof; a green run never showed it.

The suite reports each check independently, so a reader counts failures and
believes them. A check that corrupts shared state inflates one defect into a
list, and the real cause is the one nobody looks at first.

- **A check that NAVIGATES restores its starting position in a `finally`**,
  not on the success path. `run.mjs`'s file-explorer checks depend on the pane
  being inside `subdir`; put it back there whatever happened, with each
  restoring call tolerant of a page already in an unexpected state
  (`.catch(() => {})`).
- **A check that REBUILDS or RESTARTS anything** — a container, a network, a
  project — runs at the END of the suite, after everything that needs the
  state it destroys. BUG-021's network toggles are the model.
- **Prove it by making the check fail on purpose.** The cascade is invisible
  in a green run: both bugs were found only when the check was forced red.
  Any new check that navigates or rebuilds gets one deliberate failure run
  before it is trusted, and the count from that run is read — `failed=1` is
  the pass mark, `failed=9` is a second defect in the check itself.
- **Re-measure the pass count** after adding a check, and change the count
  line in `testing/styrman/README.md` in the same commit.

Same family as the global rule that a green run is evidence only when it
demonstrably hit the thing under test: here the failure, not the pass, is what
has to be observed.
