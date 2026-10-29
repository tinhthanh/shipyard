import { CheckIcon, LaptopIcon } from "lucide-react";
import { useEffect, useRef, useState } from "react";

import { Button } from "@/components/ui/button";
import { plainError } from "@/lib/errors";
import { localBoxApi, type LocalBoxStatus, useLocalBoxName } from "@/lib/local-box";
import { useStore } from "@/lib/store";
import { cn } from "@/lib/utils";
import { CommandLog } from "@/views/settings/command-log";
import { thisComputer } from "@/lib/platform";

type State = "ready" | "running" | "done" | "failed";

// UseThisMac is Add a box's way to skip a server: this Mac becomes the box.
// One button installs the berthd Shipyard carries as a launch agent that
// listens on this Mac only, and pairs with it; the log shows each step as it
// happens. Without a tailnet to list it comes first; with one, it sits below
// the tailnet's machines, compact: a second choice after them. Either way it
// is one card, the same height until it runs.
export function UseThisMac({
  status,
  compact,
  autoFocus,
  autoStart,
  className,
  onRunning,
  onPaired,
}: {
  status: LocalBoxStatus;
  compact?: boolean;
  autoFocus?: boolean;
  // Start at once: the person already chose this Mac (onboarding).
  autoStart?: boolean;
  className?: string;
  onRunning(running: boolean): void;
  onPaired(box: string): void;
}) {
  const [state, setState] = useState<State>("ready");
  const [lines, setLines] = useState<string[]>([]);
  const [error, setError] = useState<string>();
  const [paired, setPaired] = useState<string>();
  // On Windows without WSL: installing it comes first (installWSL).
  const [needsWSL, setNeedsWSL] = useState(!!status.needs_wsl);
  const [wslAsked, setWSLAsked] = useState(false);

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
  const checkWSL = async () => {
    const client = useStore.getState().client;
    if (!client) return;
    const s = await localBoxApi.status(client).catch(() => undefined);
    if (s && !s.needs_wsl) {
      setNeedsWSL(false);
      void run();
    } else {
      setError("WSL isn't installed yet. Finish its install (and restart Windows if it asks), then check again.");
    }
  };

  const run = async () => {
    const client = useStore.getState().client;
    if (!client) return;
    setState("running");
    setLines([]);
    setError(undefined);
    onRunning(true);
    try {
      const box = await localBoxApi.setUp(client, (l) => setLines((p) => [...p, l]));
      const name = box || status.name;
      useLocalBoxName.setState({ name });
      await useStore.getState().refreshStatus();
      setPaired(name);
      setState("done");
      onRunning(false);
      onPaired(name);
    } catch (err) {
      setError(plainError(err));
      setState("failed");
      onRunning(false);
    }
  };

  // Once, on arrival; StrictMode's second mount must not start it again.
  const started = useRef(false);
  useEffect(() => {
    if (!autoStart || started.current || needsWSL) return;
    started.current = true;
    void run();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [autoStart]);

  const reuse = status.installed && !status.owned;
  return (
    <section aria-labelledby="this-mac-heading" className={cn("rounded-lg border px-3.5 py-3", className)}>
      <div className="flex items-start gap-3">
        <span aria-hidden className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg border bg-background text-muted-foreground">
          <LaptopIcon className="size-4" />
        </span>
        <div className="min-w-0 flex-1">
          <h2 id="this-mac-heading" className="text-row">
            {thisComputer("Use this Mac")}
          </h2>
          <p className="mt-0.5 text-muted-foreground text-meta" aria-live="polite">
            {autoStart
              ? state === "running"
                ? status.wsl
                  ? "Setting up Ubuntu in WSL, berthd in it, and pairing with it. The first time takes a few minutes."
                  : "Installing berthd and pairing with it. This takes a few seconds."
                : state === "failed"
                  ? "It stopped before pairing; the log below says where."
                  : state === "done"
                    ? "Ready. Next, a project for your first agent."
                    : thisComputer("Agents run here while this Mac is awake.")
              : thisComputer("No server yet? Agents run here, while this Mac is awake: they pause when it sleeps. You can add a server later.")}
          </p>
        </div>
        {state === "done" ? (
          <span className="flex h-7 shrink-0 items-center gap-1 text-body text-success-foreground">
            <CheckIcon className="size-4" /> Paired
          </span>
        ) : needsWSL ? (
          <Button size="sm" variant="outline" className="shrink-0" autoFocus={autoFocus} onClick={() => void (wslAsked ? checkWSL() : installWSL())}>
            {wslAsked ? "Check again" : "Install WSL"}
          </Button>
        ) : (
          <Button size="sm" variant="outline" className="shrink-0" autoFocus={autoFocus} loading={state === "running"} onClick={() => void run()}>
            {state === "failed" ? "Try again" : thisComputer("Set up this Mac")}
          </Button>
        )}
      </div>
      {state === "ready" && needsWSL && (
        <p className="mt-2 ps-11 text-muted-foreground text-meta">
          {wslAsked
            ? "Windows is installing WSL in a window of its own. Restart Windows if it asks, then check again."
            : "Agents need Linux, which Windows runs in WSL. Installing it is the one step that needs an administrator: Windows asks, and may ask to restart."}
        </p>
      )}
      {state === "ready" && !needsWSL && !compact && (
        <p className="mt-2 ps-11 text-muted-foreground text-meta">
          {status.wsl
            ? `Shipyard sets up Ubuntu in WSL (${status.wsl}) with berthd, tmux and git: a one-time download of a few hundred MB, no password needed. `
            : reuse
              ? "Uses the berthd already installed here. "
              : "Shipyard installs berthd for your user, no password needed. "}
          {thisComputer("It listens on this Mac only, so nothing opens to your network.")}
        </p>
      )}
      {state === "ready" && error && (
        <p role="alert" className="mt-2 ps-11 text-destructive-foreground text-meta">
          {error}
        </p>
      )}
      {state !== "ready" && <CommandLog className="mt-3" lines={lines} done={state === "done"} error={error} />}
      {state === "done" && paired && (
        <p className="mt-2 text-muted-foreground text-meta">
          {thisComputer("This Mac")} is <span className="text-foreground">{paired}</span> in Shipyard.
        </p>
      )}
    </section>
  );
}
