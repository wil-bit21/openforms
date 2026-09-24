import { Component, useState, type ReactNode } from "react";
import { OpenForm } from "@openforms/react";
import "@openforms/react/styles.css";
import type { FormDefinition } from "@openforms/sdk";
import type { FormDef } from "../shared/types";

export class PreviewBoundary extends Component<{ resetKey: string; children: ReactNode }, { error: Error | null }> {
  state: { error: Error | null } = { error: null };

  static getDerivedStateFromError(error: Error) {
    return { error };
  }

  componentDidUpdate(prev: { resetKey: string }) {
    if (prev.resetKey !== this.props.resetKey && this.state.error) this.setState({ error: null });
  }

  render() {
    if (this.state.error) {
      return (
        <p role="status" className="of-ed-hint">
          Preview unavailable until the problems above are fixed ({this.state.error.message}).
        </p>
      );
    }
    return this.props.children;
  }
}

export function FormPreview({ def }: { def: FormDef }) {
  const key = JSON.stringify(def);
  const [submitted, setSubmitted] = useState<Record<string, unknown> | null>(null);
  const [nonce, setNonce] = useState(0);

  return (
    <section className="of-fe-preview" aria-label="Preview">
      <h2>Preview</h2>
      <p className="of-ed-hint">Submitting here only shows the data that would be sent; nothing is saved.</p>
      <PreviewBoundary resetKey={key}>
        <OpenForm
          key={`${key}-${nonce}`}
          definition={def as unknown as FormDefinition}
          onSubmit={async (data: Record<string, unknown>) => {
            setSubmitted(data);
          }}
        />
      </PreviewBoundary>
      {submitted ? (
        <div className="of-fe-preview__result">
          <h3>Preview submission (not saved)</h3>
          <pre>{JSON.stringify(submitted, null, 2)}</pre>
          <button
            type="button"
            className="of-ed-btn"
            onClick={() => {
              setSubmitted(null);
              setNonce((n) => n + 1);
            }}
          >
            Reset preview
          </button>
        </div>
      ) : null}
    </section>
  );
}
