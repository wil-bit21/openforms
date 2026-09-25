import { OpenForm, StatusTracker } from "@openforms/react";
import type { OpenFormsClient } from "@openforms/sdk";
import { useEffect, type ReactNode } from "react";
import { postToParent, startResizeReporting, type ParentLike } from "./embed-bridge.js";
import { statusHref, type Route } from "./route.js";

export interface AppProps {
  route: Route;
  client: OpenFormsClient;
  /** window.parent when framed, otherwise null. */
  parent: ParentLike | null;
}

function Message({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="hosted-message">
      <h1>{title}</h1>
      <p>{children}</p>
    </section>
  );
}

export function NotAvailable() {
  return (
    <Message title="This form isn't available">
      The link may be mistyped, or the form is no longer accepting responses.
    </Message>
  );
}

function FormPage({ client, slug, embed, parent }: { client: OpenFormsClient; slug: string; embed: boolean; parent: ParentLike | null }) {
  return (
    <OpenForm
      client={client}
      slug={slug}
      onLoaded={(form) => {
        document.title = form.title;
      }}
      renderLoadError={() => <NotAvailable />}
      onSubmitted={(result) => {
        if (embed && result) postToParent(parent, { type: "openforms:submitted", id: result.id, state: result.state });
      }}
      renderConfirmation={(result, form) => (
        <div className="of-confirmation" role="status">
          <p>{result?.confirmationMessage || form.settings?.confirmationMessage || "Thanks! Your response has been recorded."}</p>
          {result && (
            <p>
              <a
                href={statusHref(result.id, result.receiptToken)}
                target={embed ? "_blank" : undefined}
                rel={embed ? "noopener noreferrer" : undefined}
              >
                Track your submission
              </a>
            </p>
          )}
        </div>
      )}
    />
  );
}

function StatusPage({ client, id, token }: { client: OpenFormsClient; id: string; token: string }) {
  useEffect(() => {
    document.title = "Submission status";
  }, []);
  return <StatusTracker client={client} submissionId={id} token={token} />;
}

export function App({ route, client, parent }: AppProps) {
  useEffect(() => {
    document.documentElement.classList.toggle("of-embed", route.embed);
    if (!route.embed || !parent) return;
    return startResizeReporting(document, parent);
  }, [route.embed, parent]);

  let content: ReactNode;
  switch (route.kind) {
    case "form":
      content = <FormPage client={client} slug={route.slug} embed={route.embed} parent={parent} />;
      break;
    case "status":
      content = <StatusPage client={client} id={route.id} token={route.token} />;
      break;
    default:
      content = <Message title="Page not found">Check the link you were given and try again.</Message>;
  }

  if (route.embed) {
    return <main className="hosted hosted-embed">{content}</main>;
  }
  return (
    <div className="hosted">
      <main className="hosted-main">{content}</main>
      <footer className="hosted-footer">
        Powered by <a href="https://github.com/openforms/openforms">openforms</a>
      </footer>
    </div>
  );
}
