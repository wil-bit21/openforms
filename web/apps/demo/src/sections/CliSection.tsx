import { CodeBlock } from "../components/CodeBlock";

const CLI = `# scaffold openforms.yaml, a form and a workflow
openforms init

# check every definition offline (great in CI)
openforms validate

# push to your server: unchanged files create no new versions
export OPENFORMS_API_KEY=ofk_...
openforms push

# fail CI when someone edited a form in the UI without pulling
openforms pull --check`;

export function CliSection() {
  return (
    <section className="step" id="cli" aria-labelledby="cli-title">
      <div className="step-intro">
        <span className="step-number">5</span>
        <h2 id="cli-title">Automate from the CLI</h2>
        <p>
          The same binary is the server and the CLI. Keep definitions in git, review them in pull requests, and
          deploy them like code.
        </p>
      </div>
      <div className="card">
        <CodeBlock code={CLI} lang="bash" label="terminal" />
      </div>
    </section>
  );
}
