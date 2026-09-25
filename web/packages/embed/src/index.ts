import { findScriptSrc, init } from "./embed.js";

declare global {
  interface Window {
    OpenForms?: { scan(): void };
  }
}

// document.currentScript is only available while this script is executing, so capture it now.
const scriptSrc = findScriptSrc(document);

function start() {
  if (window.OpenForms) {
    window.OpenForms.scan();
    return;
  }
  const api = init(document, window, scriptSrc);
  window.OpenForms = { scan: api.scan };
}

if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", start, { once: true });
} else {
  start();
}
