import { Switch } from "@/components/ui/switch";
import { IS_LINUX } from "@/lib/platform";
import { setPrefs, usePrefs } from "@/lib/prefs";
import { Segmented } from "@/views/settings/controls";
import { SettingsGroup, SettingsRow } from "@/views/settings/rows";

// The floating bar (src/overlay): over every app, with each login's plan
// limits and the agents that need you or are working. macOS only.
export function FloatingBarSettings() {
  const overlay = usePrefs((p) => p.overlay);
  if (IS_LINUX) return null;
  return (
    <SettingsGroup title="Floating bar">
      <SettingsRow label="Show the floating bar" description="Stays over every app, even full screen: each login's plan limits (with the Usage & accounts plugin on), and the agents that need you or are working. Click one to open it here.">
        <Switch checked={overlay.on} onCheckedChange={(on) => setPrefs({ overlay: { ...overlay, on } })} />
      </SettingsRow>
      <SettingsRow label="Where" description="On the right edge of the screen, or around the notch at the top (the middle of the menu bar on a screen without one).">
        <Segmented
          value={overlay.place}
          options={[
            { value: "edge", label: "Right edge" },
            { value: "notch", label: "Notch" },
          ]}
          onChange={(place) => setPrefs({ overlay: { ...overlay, place } })}
        />
      </SettingsRow>
    </SettingsGroup>
  );
}
