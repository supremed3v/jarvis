// tray.ts is the SPEC-0068 system tray's Electron glue: it owns the Tray
// instance, builds its context menu from the pure model in ./trayMenu, and
// dispatches menu clicks to the handlers main.ts supplies. Only menu-template
// construction and icon bytes live in the pure module so they stay testable
// without Electron.

import { Menu, Tray, nativeImage } from "electron";
import {
  TRAY_ICON_DATA_URL,
  buildTrayMenuTemplate,
  type TrayMenuItem,
  type TrayMenuItemId,
} from "./trayMenu";

export interface TrayHandlers {
  open: () => void;
  voice: () => void;
  quit: () => void;
}

export interface JarvisTray {
  readonly tray: Tray;
  rebuild(voiceActive: boolean): void;
  destroy(): void;
}

export function createJarvisTray(handlers: TrayHandlers): JarvisTray {
  const tray = new Tray(nativeImage.createFromDataURL(TRAY_ICON_DATA_URL));
  tray.setToolTip("JARVIS");
  tray.on("double-click", handlers.open);

  let voiceActive = false;

  const dispatch = (id: TrayMenuItemId): void => {
    switch (id) {
      case "open":
        handlers.open();
        break;
      case "voice":
        handlers.voice();
        break;
      case "quit":
        handlers.quit();
        break;
    }
  };

  const rebuild = (next: boolean): void => {
    voiceActive = next;
    const items = buildTrayMenuTemplate(voiceActive).map((item: TrayMenuItem) =>
      item.type === "separator"
        ? { type: "separator" as const }
        : { label: item.label, click: (): void => dispatch(item.id) },
    );
    tray.setContextMenu(Menu.buildFromTemplate(items));
  };

  rebuild(false);

  return {
    tray,
    rebuild,
    destroy: (): void => tray.destroy(),
  };
}
