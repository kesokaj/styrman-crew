import { useEffect, useRef, useState } from "react";
import { api, tryUI, type FileContent } from "../lib/api";
import {
  notifyFileSaved,
  useEditorPane,
  type PaneFile,
} from "../lib/editorPane";
import { ConfirmDialog, IconButton } from "./ui";
import { XIcon } from "./icons";
import {
  FileEditorBody,
  FileEditorToolbar,
  useFileEditorState,
} from "./FileEditor";

/**
 * The editor as a MAIN-PANE view, sharing the pane with the terminal
 * (operator request 2026-09-01): the terminal underneath keeps running and can
 * be switched back to at any time. This replaced the centered modal file
 * viewer, which is gone.
 */
export function EditorPane({
  project,
  term,
  onShowTerminal,
}: {
  project: string;
  term: string;
  /** Switch the pane back to the terminal view. */
  onShowTerminal: () => void;
}) {
  const { state, setFile, setDirty } = useEditorPane(project, term);
  const wanted = state.file;
  const [loaded, setLoaded] = useState<{
    file: FileContent;
    req: PaneFile;
  } | null>(null);
  const [loading, setLoading] = useState(false);
  const [pending, setPending] = useState<PaneFile | null>(null);
  const [confirmClose, setConfirmClose] = useState(false);

  const st = useFileEditorState(
    loaded?.req.scope ?? { kind: "project", project },
    loaded?.file ?? EMPTY,
    notifyFileSaved, // the explorer is a sibling — signal it to re-list
  );
  // The guard reads `dirty` inside an effect that must NOT re-run when it
  // flips, or opening a file would immediately re-evaluate mid-edit.
  const dirtyRef = useRef(st.dirty);
  dirtyRef.current = st.dirty;
  // Publish it so the terminal view can show that edits are parked here.
  useEffect(() => {
    setDirty(st.dirty);
  }, [st.dirty, setDirty]);

  // Load whatever the pane store points at. A different file arriving while
  // the current one has unsaved edits parks it in `pending` behind a confirm
  // rather than silently discarding the edits.
  useEffect(() => {
    if (!wanted) return;
    const same =
      loaded &&
      loaded.req.path === wanted.path &&
      loaded.req.scope.kind === wanted.scope.kind &&
      loaded.req.scope.project === wanted.scope.project;
    if (same) return;
    if (loaded && dirtyRef.current) {
      setPending(wanted);
      return;
    }
    let stale = false;
    setLoading(true);
    void tryUI(async () => {
      try {
        const f = await api.readFile(wanted.scope, wanted.path);
        if (!stale) setLoaded({ file: f, req: wanted });
      } finally {
        if (!stale) setLoading(false);
      }
    });
    return () => {
      stale = true;
    };
    // `loaded` IS a real dependency — the `same` check above makes the effect
    // idempotent, and depending on it is what lets discardAndOpen re-run the
    // fetch by clearing `loaded` (review finding: with `loaded` excluded, and
    // the store bailing out on an identical file object, "Discard and open"
    // dead-ended on an empty pane).
  }, [wanted, loaded]);

  // The store dropped the file out from under us — it was deleted or renamed
  // in the explorer (closeFileInPane). Drop the loaded buffer too, or the pane
  // keeps showing the file at its stale path and Save would recreate it. The
  // render below reads `loaded`, not `wanted`, so clearing the store alone is
  // NOT enough.
  useEffect(() => {
    if (!wanted && loaded) setLoaded(null);
  }, [wanted, loaded]);

  const discardAndOpen = () => {
    const next = pending;
    setPending(null);
    if (!next) return;
    // Clearing `loaded` both drops the dirty buffer and re-triggers the load
    // effect above; `wanted` already points at `next`, so no store write is
    // needed (and a same-identity write would be a no-op anyway).
    setLoaded(null);
    setFile(next);
  };

  // Only reached on a real CANCEL now (ConfirmDialog's onCancel), never after
  // a confirm — which is what previously reverted the store to the original
  // file and reloaded it.
  const keepEditing = () => {
    setPending(null);
    // Point the store back at the file actually on screen, so the sidebar and
    // the pane cannot disagree about what is open.
    if (loaded) setFile(loaded.req);
  };

  // One presentation for every "nothing to edit yet" case — the loading
  // variant only swaps the copy, and both keep the way back to the terminal.
  if (!loaded) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-2 p-6 text-center">
        <p className="text-xs text-text-dim">
          {loading
            ? "Opening…"
            : "No file open. Pick one from the Files panel — it opens here."}
        </p>
        <button
          onClick={onShowTerminal}
          className="rounded-lg px-2 py-1 text-xs text-accent hover:bg-bg-hover"
        >
          Back to terminal
        </button>
      </div>
    );
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <FileEditorToolbar scope={loaded.req.scope} file={loaded.file} st={st}>
        <IconButton
          label="Close file and show the terminal"
          onClick={() => {
            if (st.dirty) {
              setPending(null);
              setConfirmClose(true);
              return;
            }
            onShowTerminal();
          }}
        >
          <XIcon />
        </IconButton>
      </FileEditorToolbar>
      <FileEditorBody file={loaded.file} st={st} />
      {pending && (
        <ConfirmDialog
          title="Discard unsaved changes?"
          body={`"${loaded.req.path}" has unsaved changes. Opening "${pending.path}" will discard them.`}
          confirmLabel="Discard and open"
          danger
          onConfirm={discardAndOpen}
          onCancel={keepEditing}
          onClose={() => setPending(null)}
        />
      )}
      {confirmClose && (
        <ConfirmDialog
          title="Discard unsaved changes?"
          body={`"${loaded.req.path}" has unsaved changes.`}
          confirmLabel="Discard"
          danger
          onConfirm={() => {
            setConfirmClose(false);
            onShowTerminal();
          }}
          onClose={() => setConfirmClose(false)}
        />
      )}
    </div>
  );
}

// Placeholder passed to the state hook before a file is loaded. The hook is a
// hook — it cannot be called conditionally — so it needs a stable stand-in.
const EMPTY: FileContent = {
  path: "",
  content: "",
  size: 0,
  binary: false,
  truncated: false,
};
