import { useState } from "react";

import { Button } from "@/components/ui/button";
import { plainError } from "@/lib/errors";
import { localBoxApi, type LocalBoxStatus } from "@/lib/local-box";
import { APP_NAME } from "@/lib/platform";
import { usePrefs } from "@/lib/prefs";
import { useStore } from "@/lib/store";

// The Windows parts of Use this computer (local-box.tsx). The box runs in
// WSL, which may need installing first, or (Labs) berthd runs on Windows
// itself. Elsewhere status says neither, and none of this shows.

export type WindowsBox = ReturnType<typeof useWindowsBox>;

export function useWindowsBox(status: LocalBoxStatus, setError: (e: string | undefined) => void) {
  // Without WSL, installing it comes first.
  const [needsWSL, setNeedsWSL] = useState(!!status.needs_wsl);
  const [wslAsked, setWSLAsked] = useState(false);
  // berthd on Windows itself rather than in WSL.
  const [native, setNative] = useState(!!status.native);
  const labs = usePrefs((p) => p.labs);

  const installWSL = async () => {
    const client = useStore.getState().client;
    if (!client) return;
    setError(undefined);
    try {
      await localBoxApi.installWSL(client);
      setWSLAsked(true);
    } catch (err) {
      setError(plainError(err));
    }
  };

  // After WSL was installed (in Windows' own window): look again.
  const checkWSL = async (onInstalled: () => void) => {
    const client = useStore.getState().client;
    if (!client) return;
    const s = await localBoxApi.status(client).catch(() => undefined);
    if (s && !s.needs_wsl) {
      setNeedsWSL(false);
      onInstalled();
    } else {
      setError("WSL isn't installed yet. Finish its install (and restart Windows if it asks), then check again.");
    }
  };

  // What setting up says while it runs, and before, where it isn't the Mac's.
  const runningLine = native
    ? "Installing berthd on Windows and pairing with it. This takes a few seconds."
    : status.wsl
      ? "Setting up Ubuntu in WSL, berthd in it, and pairing with it. The first time takes a few minutes."
      : undefined;
  const readyLine = status.native
    ? "berthd runs on Windows itself, and agents in PowerShell. "
    : status.wsl
      ? `${APP_NAME} sets up Ubuntu in WSL (${status.wsl}) with berthd, tmux and git: a one-time download of a few hundred MB, no password needed. `
      : undefined;
  const offersNative = labs && !!status.native_available && !status.installed;

  return { needsWSL, wslAsked, native, setNative, installWSL, checkWSL, runningLine, readyLine, offersNative };
}

// InstallWSLButton stands in for Set up while WSL is missing.
export function InstallWSLButton({ win, autoFocus, onInstalled }: { win: WindowsBox; autoFocus?: boolean; onInstalled(): void }) {
  return (
    <Button size="sm" variant="outline" className="shrink-0" autoFocus={autoFocus} onClick={() => void (win.wslAsked ? win.checkWSL(onInstalled) : win.installWSL())}>
      {win.wslAsked ? "Check again" : "Install WSL"}
    </Button>
  );
}

// WindowsBoxNotes go under the card before it runs: what installing WSL
// takes, the Labs way, and what went wrong with either.
export function WindowsBoxNotes({ win, error, onNative }: { win: WindowsBox; error?: string; onNative(): void }) {
  return (
    <>
      {win.needsWSL && (
        <p className="mt-2 ps-11 text-muted-foreground text-meta">
          {win.wslAsked
            ? "Windows is installing WSL in a window of its own. Restart Windows if it asks, then check again."
            : "Agents need Linux, which Windows runs in WSL. Installing it is the one step that needs an administrator: Windows asks, and may ask to restart."}
        </p>
      )}
      {win.offersNative && (
        <p className="mt-1 ps-11 text-meta">
          <Button variant="link" size="sm" className="h-auto p-0 text-meta" onClick={onNative}>
            Or run agents on Windows itself, without WSL (Labs)
          </Button>
        </p>
      )}
      {error && (
        <p role="alert" className="mt-2 ps-11 text-destructive-foreground text-meta">
          {error}
        </p>
      )}
    </>
  );
}
