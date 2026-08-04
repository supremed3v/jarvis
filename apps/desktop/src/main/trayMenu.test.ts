import { test } from "node:test";
import assert from "node:assert";
import { TRAY_ICON_DATA_URL, buildTrayMenuTemplate, type TrayMenuItem } from "./trayMenu";

function describe(items: TrayMenuItem[]): string[] {
  return items.map((item) => (item.type === "separator" ? "---" : `${item.id}:${item.label}`));
}

test("menu template lists the quick actions in order", () => {
  assert.deepStrictEqual(describe(buildTrayMenuTemplate(false)), [
    "open:Open Application",
    "settings:Settings",
    "agents:Agents",
    "voice:Start Voice Mode",
    "---",
    "quit:Exit",
  ]);
});

test("voice item toggles its label with the voice mode state", () => {
  const idle = buildTrayMenuTemplate(false).find(
    (item): item is Extract<TrayMenuItem, { type: "normal" }> => item.type === "normal" && item.id === "voice",
  );
  assert.ok(idle, "voice item missing from idle menu");
  assert.strictEqual(idle.label, "Start Voice Mode");

  const active = buildTrayMenuTemplate(true).find(
    (item): item is Extract<TrayMenuItem, { type: "normal" }> => item.type === "normal" && item.id === "voice",
  );
  assert.ok(active, "voice item missing from active menu");
  assert.strictEqual(active.label, "Stop Voice Mode");
});

test("every normal item carries one of the dispatcher's known ids", () => {
  const ids = new Set(["open", "settings", "agents", "voice", "quit"]);
  for (const item of buildTrayMenuTemplate(false)) {
    if (item.type === "separator") {
      continue;
    }
    assert.ok(ids.has(item.id), `unexpected menu item id ${item.id}`);
  }
});

test("tray icon is an embedded PNG data URL", () => {
  assert.match(TRAY_ICON_DATA_URL, /^data:image\/png;base64,[A-Za-z0-9+/=]+$/);
});
