// Plugin icons for Settings → Plugins. A plugin in ~/.berth/plugins, or one
// the listing has, gets its own: its manifest's "icon", which the agent
// reads and hands over as a data: URL (internal/agent/pluginicon.go). The
// built-ins ship with the app and get one of the app's own icons. Anything
// else gets the first letter of its name on a colour its id always gets.
import {
  ActivityIcon,
  ChartColumnIcon,
  CircleDotIcon,
  DiffIcon,
  GaugeIcon,
  GitCompareIcon,
  GitPullRequestIcon,
  type LucideIcon,
  MessageSquareTextIcon,
  NotebookPenIcon,
  ServerIcon,
} from "lucide-react";

import { isMock } from "@/hooks/use-berth-connection";
import { cn } from "@/lib/utils";

const builtinIcons: Record<string, LucideIcon> = {
  activity: ActivityIcon,
  "box-monitor": GaugeIcon,
  "dev-servers": ServerIcon,
  diff: DiffIcon,
  "git-changes": GitCompareIcon,
  issues: CircleDotIcon,
  notes: NotebookPenIcon,
  prompts: MessageSquareTextIcon,
  "pull-request": GitPullRequestIcon,
  usage: ChartColumnIcon,
};

// iconURL is a plugin's icon as the agent gave it: only ever a data: image,
// so nothing a listing says can make the app load an address.
export function iconURL(p: { id: string }): string | undefined {
  const u = (p as { icon_url?: unknown }).icon_url;
  if (typeof u === "string" && u.startsWith("data:image/")) return u;
  return isMock() ? mockIcons[p.id] : undefined;
}

export function PluginMark({ plugin: p, builtin, size = "md" }: { plugin: { id: string; name: string }; builtin?: boolean; size?: "md" | "lg" }) {
  const box = size === "lg" ? "size-10 rounded-xl" : "size-8 rounded-lg";
  const src = iconURL(p);
  if (src)
    return (
      <span aria-hidden className={cn("flex shrink-0 items-center justify-center overflow-hidden border border-border/60 bg-background", box)}>
        <img src={src} alt="" className="size-full object-contain" draggable={false} />
      </span>
    );
  const Icon = builtin ? builtinIcons[p.id] : undefined;
  if (Icon)
    return (
      <span aria-hidden className={cn("flex shrink-0 items-center justify-center bg-muted text-foreground", box)}>
        <Icon className={size === "lg" ? "size-5" : "size-4"} />
      </span>
    );
  let h = 0;
  for (const c of p.id) h = (h * 31 + c.charCodeAt(0)) % 360;
  return (
    <span
      aria-hidden
      className={cn("flex shrink-0 select-none items-center justify-center font-semibold text-white", box, size === "lg" ? "text-section" : "text-body")}
      style={{ background: `oklch(0.58 0.13 ${h})` }}
    >
      {(p.name.trim()[0] ?? p.id[0] ?? "?").toUpperCase()}
    </span>
  );
}

// Mock mode's plugins have icons of their own, so the page shows both an
// icon and the letter it falls back to.
const svg = (bg: string, body: string) =>
  `data:image/svg+xml;base64,${btoa(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" rx="7" fill="${bg}"/>${body}</svg>`)}`;
const mockIcons: Record<string, string> = {
  "hello-ports": svg("#0ea5e9", `<circle cx="16" cy="16" r="7" fill="none" stroke="#fff" stroke-width="2.5"/><circle cx="16" cy="16" r="2.5" fill="#fff"/>`),
  "acme-standup": svg("#f59e0b", `<path d="M9 21h14M11 16h10M13 11h6" stroke="#fff" stroke-width="2.5" stroke-linecap="round"/>`),
  leadline: svg("#22c55e", `<path d="M8 22l6-7 4 4 6-9" fill="none" stroke="#fff" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"/>`),
  database: svg("#6366f1", `<ellipse cx="16" cy="10" rx="7" ry="3" fill="#fff"/><path d="M9 10v12c0 1.7 3.1 3 7 3s7-1.3 7-3V10" fill="none" stroke="#fff" stroke-width="2.2"/><path d="M9 16c0 1.7 3.1 3 7 3s7-1.3 7-3" fill="none" stroke="#fff" stroke-width="2.2"/>`),
};
