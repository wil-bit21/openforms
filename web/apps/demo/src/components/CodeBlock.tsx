import { useEffect, useState } from "react";
import { highlight, type Lang } from "../highlight";

export function CodeBlock({ code, lang, label }: { code: string; lang: Lang; label?: string }) {
  const [html, setHtml] = useState<string | null>(null);

  useEffect(() => {
    let live = true;
    setHtml(null);
    highlight(code, lang)
      .then((h) => {
        if (live) setHtml(h);
      })
      .catch(() => {
        /* keep the plain fallback */
      });
    return () => {
      live = false;
    };
  }, [code, lang]);

  return (
    <figure className="code">
      {label && <figcaption>{label}</figcaption>}
      {html ? (
        <div className="code-body" dangerouslySetInnerHTML={{ __html: html }} />
      ) : (
        <pre className="code-body">
          <code>{code}</code>
        </pre>
      )}
    </figure>
  );
}
