import type { PluginOnBox } from "@/lib/api";

// What Settings → Plugins says of a plugin's tools on a box, and what it
// offers there (internal/agent/boxplugins.go). Kept apart from the view so
// it is tested on its own.

export interface BoxRowView {
  // On the box (the switch is on).
  on: boolean;
  // The switch can be used.
  canToggle: boolean;
  // What the row says after the box's name.
  note: string;
  tone: "muted" | "success" | "warning" | "error";
  // A button beside it: send the version allowed here, or start the
  // plugin's setup on the box's screen.
  action?: "update" | "setup";
}

export function boxRowView(row: PluginOnBox): BoxRowView {
  switch (row.state) {
    case "off":
      return { on: false, canToggle: true, note: "Not on this box", tone: "muted" };
    case "outdated":
      return { on: true, canToggle: true, note: row.version ? `Has ${row.version}, not the version allowed here` : "Has another version", tone: "warning", action: "update" };
    case "on":
      if (row.setup && row.setup.known && !row.setup.ready) {
        return { on: true, canToggle: true, note: row.setup.label ? `Needs setup on the box: ${row.setup.label}` : "Needs one more step on the box", tone: "warning", action: "setup" };
      }
      return { on: true, canToggle: true, note: row.setup?.known ? "On the box, and set up" : "On the box", tone: "success" };
    case "unsupported":
      return { on: false, canToggle: false, note: row.error ?? "Doesn't run on this box", tone: "muted" };
    case "old_berthd":
      return { on: false, canToggle: false, note: "Update this box's berthd to put plugins on it", tone: "muted" };
    default:
      return { on: false, canToggle: false, note: row.error ?? "Couldn't reach the box", tone: "error" };
  }
}
