import { useEffect, useState } from "react";
import { OpenFormsError } from "@openforms/sdk";
import { client } from "./client";

export type DemoStatus =
  | { kind: "loading" }
  | { kind: "offline" }
  | { kind: "not-seeded"; demoMode: boolean }
  | { kind: "ready"; demoMode: boolean };

export const DEMO_FORM_SLUG = "job-application";

export function useDemoStatus(): DemoStatus {
  const [status, setStatus] = useState<DemoStatus>({ kind: "loading" });

  useEffect(() => {
    let cancelled = false;
    const set = (s: DemoStatus) => {
      if (!cancelled) setStatus(s);
    };
    (async () => {
      let demoMode: boolean;
      try {
        demoMode = (await client.getPublicConfig()).demo;
      } catch {
        set({ kind: "offline" });
        return;
      }
      try {
        await client.getPublicForm(DEMO_FORM_SLUG);
        set({ kind: "ready", demoMode });
      } catch (e) {
        set(e instanceof OpenFormsError && e.status === 404 ? { kind: "not-seeded", demoMode } : { kind: "offline" });
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  return status;
}
