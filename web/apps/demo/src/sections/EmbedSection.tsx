import { useEffect, useRef, useState } from "react";
import { CodeBlock } from "../components/CodeBlock";

export function EmbedSection({ enabled }: { enabled: boolean }) {
  const host = useRef<HTMLDivElement>(null);
  const injected = useRef(false);
  const [copied, setCopied] = useState(false);
  const snippet = `<div data-openforms="contact"></div>\n<script src="${window.location.origin}/embed.js" async></script>`;

  useEffect(() => {
    if (!enabled || injected.current || !host.current) return;
    injected.current = true;
    const script = document.createElement("script");
    script.src = "/embed.js";
    script.async = true;
    host.current.appendChild(script);
  }, [enabled]);

  async function copy() {
    try {
      await navigator.clipboard.writeText(snippet);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    } catch {
      setCopied(false);
    }
  }

  return (
    <section className="step" id="embed" aria-labelledby="embed-title">
      <div className="step-intro">
        <span className="step-number">4</span>
        <h2 id="embed-title">Embed</h2>
        <p>
          Not rendering forms yourself? Drop two lines into any page. The hosted form loads in an auto-resizing
          iframe and emits an <code>openforms:submitted</code> event.
        </p>
      </div>
      <div className="embed-grid">
        <div className="card">
          <CodeBlock code={snippet} lang="html" label="index.html" />
          <button type="button" className="button" onClick={() => void copy()}>
            {copied ? "Copied" : "Copy snippet"}
          </button>
        </div>
        <div className="card" ref={host}>
          {enabled ? (
            <div data-openforms="contact" />
          ) : (
            <p className="muted">The live embed appears once the demo bundle is seeded.</p>
          )}
        </div>
      </div>
    </section>
  );
}
