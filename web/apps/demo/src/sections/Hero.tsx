export function Hero() {
  return (
    <header className="hero">
      <div className="wrap">
        <p className="eyebrow">openforms · live demo</p>
        <h1>
          Forms as code.
          <br />
          Workflows built in.
        </h1>
        <p className="lede">
          openforms is an open-source, self-hosted form platform for developers. Define forms and their review
          workflows in YAML, push them with a CLI, and move every submission through guarded states, with
          webhooks, emails and a full audit trail.
        </p>
        <div className="hero-actions">
          <a className="button primary" href="#collect">
            Try it below
          </a>
          <a className="button" href="https://github.com/openforms/openforms">
            GitHub
          </a>
          <a className="button" href="https://github.com/openforms/openforms/tree/main/docs">
            Docs
          </a>
        </div>
      </div>
    </header>
  );
}
