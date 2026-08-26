import { createFileRoute } from "@tanstack/react-router";

export const Route = createFileRoute("/")({ component: Home });

function Home() {
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-2 text-text-dim">
      <div className="text-4xl">⛵</div>
      <p className="text-sm">Pick a project, or create one.</p>
      <p className="max-w-sm text-center text-xs">
        Every project runs in its own microVM with its own Docker daemon. The
        agent CLI in the terminal is the conversation.
      </p>
    </div>
  );
}
