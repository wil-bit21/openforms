import { OpenForm } from "@openforms/react";
import { client } from "../client";
import { Notice } from "../components/Notice";
import { DEMO_FORM_SLUG, type DemoStatus } from "../useDemoStatus";

export interface Submitted {
  id: string;
  receiptToken: string;
}

export function CollectSection({ status, onSubmitted }: { status: DemoStatus; onSubmitted: (s: Submitted) => void }) {
  return (
    <section className="step" id="collect" aria-labelledby="collect-title">
      <div className="step-intro">
        <span className="step-number">2</span>
        <h2 id="collect-title">Collect</h2>
        <p>
          This form is rendered headlessly with <code>@openforms/react</code>, straight from the definition
          above. Conditional fields, validation and the submit label all come from the YAML. Pick “Designer” to
          see the portfolio field appear.
        </p>
      </div>
      <div className="card form-card">
        {status.kind === "loading" && <p className="muted">Connecting to the openforms server…</p>}
        {status.kind === "offline" && (
          <Notice title="Server unreachable" tone="warn">
            The demo could not reach the openforms API. Start the server with{" "}
            <code>docker compose --profile app up</code> and reload this page.
          </Notice>
        )}
        {status.kind === "not-seeded" && (
          <Notice title="Live demo not enabled">
            Run <code>openforms seed --demo</code> to enable the live demo.
          </Notice>
        )}
        {status.kind === "ready" && (
          <OpenForm
            client={client}
            slug={DEMO_FORM_SLUG}
            onSubmitted={(result) => result && onSubmitted({ id: result.id, receiptToken: result.receiptToken })}
          />
        )}
      </div>
    </section>
  );
}
