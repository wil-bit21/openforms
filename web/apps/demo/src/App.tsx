import { useState } from "react";
import { useDemoStatus } from "./useDemoStatus";
import { Hero } from "./sections/Hero";
import { DefineSection } from "./sections/DefineSection";
import { CollectSection, type Submitted } from "./sections/CollectSection";
import { TrackSection } from "./sections/TrackSection";
import { EmbedSection } from "./sections/EmbedSection";
import { CliSection } from "./sections/CliSection";

export function App() {
  const status = useDemoStatus();
  const [submitted, setSubmitted] = useState<Submitted | null>(null);
  const ready = status.kind === "ready";

  return (
    <>
      <Hero />
      <main className="wrap">
        <DefineSection />
        <CollectSection status={status} onSubmitted={setSubmitted} />
        <TrackSection submitted={submitted} demoMode={ready && status.demoMode} />
        <EmbedSection enabled={ready} />
        <CliSection />
      </main>
      <footer className="footer wrap">
        <p>
          openforms is open source.{" "}
          <a href="https://github.com/openforms/openforms">Star it on GitHub</a> ·{" "}
          <a href="/admin">Admin</a>
        </p>
      </footer>
    </>
  );
}
