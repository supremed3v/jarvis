// trayMenu.ts holds the SPEC-0068 system tray's menu model and icon asset.
// It is a pure module (no electron import) so node:test can exercise the
// menu template without a running Electron instance; the electron glue lives
// in ./tray, and main.ts wires the two together.

// TRAY_ICON_DATA_URL is the tray's icon: a 32x32 PNG (cyan orb on a dark
// center, transparent background) matching the renderer's voice-orb palette,
// embedded as a data URL so the compiled main bundle is self-contained.
export const TRAY_ICON_DATA_URL =
  "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAACAAAAAgCAYAAABzenr0AAAAAXNSR0IArs4c6QAAAARnQU1BAACxjwv8YQUAAAAJcEhZcwAADsMAAA7DAcdvqGQAAAHzSURBVFhH7ZY9LENRFMev+ihKmHxNXWwiIhESi5hsjEaLxGhjIBEGsegiTNJBQsKAySKRiEREotE+YdJJDAaJhekcObevr++d8/q+qoOk/+Q39f3P//bec897StVUU1Q9wpjK4YSDqotCnyCtDPhUBqIgB9/KgCOVxRlurUwG9igDTkWgJ3ClDBzipcJLbzW8y4AA0I7kYJaXDC46V140CpEW8YzJyP+co3sj7HGEPnMfcpDhEeUVcOsbz18wfniPTSeP4rcyzPEod9FVkmYLCuwanca+vkGLnoFJjKdvxLNO4JZHSeWx2TwzlwKFcHswp3X7THgcUG95iq4dN5nUPfxg9/CUCHXsRP84xq4/hNfC90bQFOMmE79/X6Rl50J4bQtY5JFOZWFBmEwSW8cizI32pV3hLS0A13ikU7RF3GRCTcbD3KCFcq9FFpd5pFMeVzB294W9yRERyGm4eBVeGz5XMYOdLiYLv2PomN8QHkaAiVh4k3GjRdvKvutOdM6t6pvCny8BeR7lLo9GLFJ/+YaJ9QPdcLQgmor8GcETbPKo8qLZzQtUAr3YaMgFlkczRsSn+dxEQ0MWigCkeOngonsrCoYB9njJ8NJfvWE/TuBTN/OfiRpI7wbkZZgNvVBI6XlSNdEwocXQXLdDb9Ka/pt+AXPkz2ylL3H0AAAAAElFTkSuQmCC";

export type TrayMenuItemId = "open" | "settings" | "agents" | "memory" | "voice" | "quit";

export interface TrayMenuSeparator {
  type: "separator";
}

export interface TrayMenuCommand {
  id: TrayMenuItemId;
  label: string;
  type: "normal";
}

export type TrayMenuItem = TrayMenuSeparator | TrayMenuCommand;

// buildTrayMenuTemplate returns the tray's context-menu items. The voice item
// doubles as a state indicator: it reads "Start Voice Mode" when the voice
// session lifecycle is stopped and "Stop Voice Mode" once it is running, so
// the tray's single action both controls and reflects SPEC-0068's voice mode.
// Settings (SPEC-0069) opens the settings window; Agents (SPEC-0070) opens the
// agent management dashboard; Memory (SPEC-0071) opens the memory viewer.
export function buildTrayMenuTemplate(voiceActive: boolean): TrayMenuItem[] {
  return [
    { id: "open", label: "Open Application", type: "normal" },
    { id: "settings", label: "Settings", type: "normal" },
    { id: "agents", label: "Agents", type: "normal" },
    { id: "memory", label: "Memory", type: "normal" },
    { id: "voice", label: voiceActive ? "Stop Voice Mode" : "Start Voice Mode", type: "normal" },
    { type: "separator" },
    { id: "quit", label: "Exit", type: "normal" },
  ];
}
