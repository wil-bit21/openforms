import { useState } from "react";
import { SOURCES } from "../sources";
import { CodeBlock } from "../components/CodeBlock";

export function DefineSection() {
  const [active, setActive] = useState(0);
  return (
    <section className="step" id="define" aria-labelledby="define-title">
      <div className="step-intro">
        <span className="step-number">1</span>
        <h2 id="define-title">Define</h2>
        <p>
          A form and its workflow are two small YAML files in your repo. These are the files running behind
          this page. The workflow gives each submission its states, who may move it and what happens when it
          moves.
        </p>
      </div>
      <div className="card">
        <div role="tablist" aria-label="Definition files" className="tabs">
          {SOURCES.map((s, i) => (
            <button
              key={s.file}
              role="tab"
              id={`tab-${i}`}
              aria-selected={i === active}
              aria-controls={`panel-${i}`}
              tabIndex={i === active ? 0 : -1}
              className="tab"
              onClick={() => setActive(i)}
            >
              {s.file}
            </button>
          ))}
        </div>
        <div role="tabpanel" id={`panel-${active}`} aria-labelledby={`tab-${active}`}>
          <CodeBlock code={SOURCES[active].code} lang="yaml" />
        </div>
      </div>
    </section>
  );
}
