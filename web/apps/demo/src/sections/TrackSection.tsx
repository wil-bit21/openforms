import { useState } from "react";
import { StatusTracker } from "@openforms/react";
import { client } from "../client";
import type { Submitted } from "./CollectSection";
import { ReviewerPanel } from "./ReviewerPanel";

export function TrackSection({ submitted, demoMode }: { submitted: Submitted | null; demoMode: boolean }) {
  const [refreshKey, setRefreshKey] = useState(0);
  return (
    <section className="step" id="track" aria-labelledby="track-title">
      <div className="step-intro">
        <span className="step-number">3</span>
        <h2 id="track-title">Track &amp; review</h2>
        <p>
          Every submission gets a private status link. Act as a reviewer or hiring manager on the right and
          watch the respondent’s tracker update. Each move is guarded, audited, and can send email or fire
          webhooks.
        </p>
      </div>
      {!submitted ? (
        <div className="card empty">
          <p className="muted">Submit the application above to watch it move through the hiring workflow.</p>
        </div>
      ) : (
        <div className="track-grid">
          <div className="card">
            <h3>What the applicant sees</h3>
            <StatusTracker
              key={refreshKey}
              client={client}
              submissionId={submitted.id}
              token={submitted.receiptToken}
              pollMs={3000}
            />
          </div>
          <div className="card">
            <ReviewerPanel
              submissionId={submitted.id}
              demoMode={demoMode}
              onTransitioned={() => setRefreshKey((k) => k + 1)}
            />
            <a className="inbox-link" href={`/admin/submissions/${submitted.id}`}>
              Open in the admin inbox
            </a>
          </div>
        </div>
      )}
    </section>
  );
}
