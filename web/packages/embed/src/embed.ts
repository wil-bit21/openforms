// embed.js — turns <div data-openforms="slug"> into an auto-resizing iframe of the hosted form.
export const RESIZE = "openforms:resize";
export const SUBMITTED = "openforms:submitted";
export const MOUNTED_ATTR = "data-openforms-mounted";

const INITIAL_HEIGHT = 480;
const MAX_HEIGHT = 20000;

export function originFromScript(src: string | null | undefined, fallback: string): string {
  if (!src) return fallback;
  try {
    return new URL(src, fallback).origin;
  } catch {
    return fallback;
  }
}

export function findScriptSrc(doc: Document): string | null {
  const current = doc.currentScript as HTMLScriptElement | null;
  if (current?.src) return current.src;
  const tag = doc.querySelector<HTMLScriptElement>('script[src*="/embed.js"]');
  return tag?.src || null;
}

export function mount(container: HTMLElement, origin: string): HTMLIFrameElement | null {
  const slug = container.getAttribute("data-openforms")?.trim();
  if (!slug || container.hasAttribute(MOUNTED_ATTR)) return null;

  const iframe = container.ownerDocument.createElement("iframe");
  iframe.src = `${origin}/f/${encodeURIComponent(slug)}?embed=1`;
  iframe.title = container.getAttribute("data-openforms-title") || `Form: ${slug}`;
  iframe.setAttribute("loading", "lazy");
  iframe.style.width = "100%";
  iframe.style.border = "0";
  iframe.style.display = "block";
  iframe.style.height = `${INITIAL_HEIGHT}px`;

  container.setAttribute(MOUNTED_ATTR, "");
  container.appendChild(iframe);
  return iframe;
}

interface EmbedMessage {
  type?: unknown;
  height?: unknown;
  id?: unknown;
  state?: unknown;
}

export function handleMessage(event: MessageEvent, origin: string, doc: Document): void {
  if (event.origin !== origin) return;
  const data = event.data as EmbedMessage | null;
  if (data === null || typeof data !== "object") return;

  const iframe = Array.from(doc.querySelectorAll<HTMLIFrameElement>(`[${MOUNTED_ATTR}] > iframe`)).find(
    (frame) => frame.contentWindow !== null && frame.contentWindow === event.source,
  );
  if (!iframe) return;

  if (data.type === RESIZE) {
    const h = data.height;
    if (typeof h === "number" && Number.isFinite(h) && h > 0 && h <= MAX_HEIGHT) {
      iframe.style.height = `${Math.ceil(h)}px`;
    }
  } else if (data.type === SUBMITTED) {
    iframe.parentElement?.dispatchEvent(
      new CustomEvent(SUBMITTED, { bubbles: true, detail: { id: data.id, state: data.state } }),
    );
  }
}

export function init(
  doc: Document = document,
  win: Window = window,
  scriptSrc: string | null = findScriptSrc(doc),
): { origin: string; scan(): void; destroy(): void } {
  const origin = originFromScript(scriptSrc, win.location.origin);
  const scan = () => {
    doc.querySelectorAll<HTMLElement>("[data-openforms]").forEach((el) => {
      mount(el, origin);
    });
  };
  const listener = (event: MessageEvent) => handleMessage(event, origin, doc);
  win.addEventListener("message", listener);
  scan();
  return { origin, scan, destroy: () => win.removeEventListener("message", listener) };
}
