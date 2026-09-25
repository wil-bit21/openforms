export interface ParentLike {
  postMessage(message: unknown, targetOrigin: string): void;
}

export function postToParent(parent: ParentLike | null, message: { type: string; [key: string]: unknown }): void {
  // The embedding page's origin is unknown; messages carry no secrets (height, submission id, state).
  parent?.postMessage(message, "*");
}

export function startResizeReporting(
  doc: Document,
  parent: ParentLike,
  RO: typeof ResizeObserver | undefined = globalThis.ResizeObserver,
): () => void {
  let last = -1;
  const report = () => {
    const height = Math.ceil(doc.documentElement.scrollHeight);
    if (height === last) return;
    last = height;
    postToParent(parent, { type: "openforms:resize", height });
  };
  report();
  if (RO === undefined) {
    const id = setInterval(report, 500);
    return () => clearInterval(id);
  }
  const observer = new RO(report);
  observer.observe(doc.body);
  return () => observer.disconnect();
}
