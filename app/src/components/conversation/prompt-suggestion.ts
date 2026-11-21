import { useEffect, useState } from "react";
import type { KeyboardEvent } from "react";

import { useStore } from "@/lib/store";

// Claude's guess at the next prompt (fork; berthd: promptsuggest.go): after
// each turn of a structured Claude session, the Reply box shows it in grey
// where its placeholder goes, as Claude Code's own prompt does, and Tab
// puts it in to send or change. Typing anything else hides it. Boxes that
// list "prompt.suggestions" have it; on others nothing shows.

// How long one read waits for the suggestion, which comes a second or so
// after the turn ends.
const WAIT = "15s";

export function usePromptSuggestion(box: string | undefined, session: string | undefined, idle: boolean, typed: string) {
  const client = useStore((s) => s.client);
  const takes = useStore((s) => !!box && !!s.boxes[box]?.info?.capabilities?.includes("prompt.suggestions"));
  const key = box && session ? `${box}\u0000${session}` : "";
  const [got, setGot] = useState<{ key: string; text: string }>();

  useEffect(() => {
    // Working again: the last turn's guess is stale.
    if (!idle) setGot(undefined);
    if (!idle || !takes || !client || !box || !session) return;
    const abort = new AbortController();
    client
      .box<{ suggestion?: string }>(box, "GET", `sessions/${encodeURIComponent(session)}/suggestion?wait=${WAIT}`, undefined, abort.signal)
      .then((r) => setGot({ key, text: r.suggestion?.trim() ?? "" }))
      .catch(() => {
        // Away, or an older box: no suggestion, as before.
      });
    return () => abort.abort();
  }, [idle, takes, client, box, session, key]);

  const text = idle && !typed && got?.key === key ? got.text : "";
  return {
    text,
    // The placeholder to show: the suggestion, when there is one.
    placeholder: (fallback?: string) => text || fallback,
    // Tab takes it; says whether the key was used.
    onKeyDown(e: KeyboardEvent, setText: (t: string) => void): boolean {
      if (e.key !== "Tab" || e.shiftKey || e.altKey || e.ctrlKey || e.metaKey || !text) return false;
      e.preventDefault();
      setText(text);
      return true;
    },
  };
}
