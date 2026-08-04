interface JarvisAPI {
  getVersion: () => string;
  platform: string;
  runtime: {
    ping: () => Promise<string>;
  };
}

declare global {
  interface Window {
    jarvis: JarvisAPI;
  }
}

document.addEventListener("DOMContentLoaded", () => {
  const statusEl = document.getElementById("status");
  const versionEl = document.getElementById("version");

  if (window.jarvis) {
    if (statusEl) statusEl.textContent = "Ready";
    if (versionEl) versionEl.textContent = `Electron ${window.jarvis.getVersion()} — ${window.jarvis.platform}`;
  } else {
    if (statusEl) statusEl.textContent = "Preload bridge unavailable";
  }
});

export {};
