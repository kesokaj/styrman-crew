import { createFileRoute } from "@tanstack/react-router";
import { CrewMark } from "../components/icons";

export const Route = createFileRoute("/")({ component: Home });

function Home() {
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-3 p-6 text-text-dim">
      <CrewMark className="h-12 w-12" />
      <p className="text-sm text-text">Pick a project, or create one.</p>
      <p className="max-w-sm text-center text-xs">
        Every project runs in its own microVM with its own Docker daemon. The
        agent CLI in the terminal is the conversation.
      </p>
    </div>
  );
}
