import { useState } from "react";
import { createPortal } from "react-dom";
import { api } from "../lib/api";

/**
 * Deleting a project destroys its microVM. Instead of alert/confirm popups,
 * the user must type "delete" (operator request). Wiping the files is a
 * separate explicit choice.
 */
export function DeleteProjectDialog({
  projectId,
  projectName,
  onClose,
  onDeleted,
}: {
  projectId: string;
  projectName: string;
  onClose: () => void;
  onDeleted: () => void;
}) {
  const [typed, setTyped] = useState("");
  const [wipe, setWipe] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  const armed = typed.trim().toLowerCase() === "delete";

  const run = async () => {
    setBusy(true);
    setError(undefined);
    try {
      await api.deleteProject(projectId, wipe);
      onDeleted();
    } catch (e) {
      setError((e as Error).message);
      setBusy(false);
    }
  };

  // Portal: a transformed ancestor (the sidebar drawer) would otherwise
  // become the containing block for position:fixed and shrink the dialog.
  return createPortal(
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
      onClick={onClose}
    >
      <div
        className="w-96 max-w-full rounded-3xl bg-bg-raised p-5 shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="mb-1 text-sm font-semibold text-danger">
          Delete project “{projectName}”
        </h2>
        <p className="mb-3 text-xs text-text-dim">
          This destroys the project's microVM and removes it from styrman.
        </p>
        <label className="mb-3 flex items-start gap-2 text-xs">
          <input
            type="checkbox"
            checked={wipe}
            onChange={(e) => setWipe(e.target.checked)}
            className="mt-0.5"
          />
          <span>
            Also delete its files on disk — workspace, git history, docker disk,
            and transcripts.
            <span className="text-danger"> Irreversible.</span>
            {!wipe && (
              <span className="text-text-dim">
                {" "}
                (Unchecked: files stay in your home.)
              </span>
            )}
          </span>
        </label>
        <label
          className="mb-1 block text-xs text-text-dim"
          htmlFor="del-confirm"
        >
          Type <span className="font-mono font-semibold text-text">delete</span>{" "}
          to confirm
        </label>
        <input
          id="del-confirm"
          autoFocus
          value={typed}
          onChange={(e) => setTyped(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && armed && !busy && run()}
          className="mb-3 w-full rounded-md border border-border bg-bg px-2 py-1.5 font-mono outline-none focus:border-danger"
        />
        {error && <p className="mb-3 text-xs text-danger">{error}</p>}
        <div className="flex justify-end gap-2 text-xs">
          <button
            onClick={onClose}
            className="rounded-md px-3 py-1.5 hover:bg-bg-hover"
          >
            Cancel
          </button>
          <button
            onClick={run}
            disabled={!armed || busy}
            className="rounded-md bg-danger px-3 py-1.5 font-medium text-on-danger disabled:opacity-40"
          >
            {busy ? "Deleting…" : "Delete project"}
          </button>
        </div>
      </div>
    </div>
  , document.body);
}
