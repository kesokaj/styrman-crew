import { useRef, useState } from "react";
import { createFileRoute } from "@tanstack/react-router";
import { api } from "../lib/api";
import { usePolled } from "../lib/hooks";
import { RightRail } from "../components/RightRail";
import { TerminalPane } from "../components/TerminalPane";

export const Route = createFileRoute("/p/$project/$conv")({
  component: ConversationPage,
});

function ConversationPage() {
  const { project, conv } = Route.useParams();
  const { data, refresh } = usePolled(
    () => api.listConversations(project),
    10000,
  );
  const record = data?.conversations.find((c) => c.id === conv);
  const [editing, setEditing] = useState(false);
  const [title, setTitle] = useState("");

  const savingRef = useRef(false);
  const saveTitle = async () => {
    if (savingRef.current) return; // Enter triggers blur → avoid double PATCH
    savingRef.current = true;
    setEditing(false);
    if (title.trim() && title.trim() !== record?.title) {
      await api.renameConversation(project, conv, title.trim()).catch(() => {});
      refresh();
    }
    savingRef.current = false;
  };

  return (
    <>
      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-10 shrink-0 items-center gap-2 border-b border-border bg-bg-raised px-3 text-xs">
          <span className="text-text-dim">{project}</span>
          <span className="text-text-dim">/</span>
          {editing ? (
            <input
              autoFocus
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              onBlur={saveTitle}
              onKeyDown={(e) => e.key === "Enter" && saveTitle()}
              className="rounded border border-border bg-bg px-1.5 py-0.5 outline-none focus:border-accent"
            />
          ) : (
            <button
              className="font-medium hover:underline"
              title="Rename conversation"
              onClick={() => {
                setTitle(record?.title ?? "");
                setEditing(true);
              }}
            >
              {record?.title ?? `Conversation ${conv}`}
            </button>
          )}
          {record?.live && (
            <span
              className="h-1.5 w-1.5 rounded-full bg-ok"
              title="live session"
            />
          )}
        </header>
        <div className="min-h-0 flex-1">
          <TerminalPane
            key={`${project}/${conv}`}
            target={{ kind: "conv", project, id: conv }}
            onExited={refresh}
          />
        </div>
      </div>
      <RightRail project={project} />
    </>
  );
}
