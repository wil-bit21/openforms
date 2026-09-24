import { useQuery } from "@tanstack/react-query";
import { client } from "../api";
import "./DemoCredentials.css";

export const DEMO_PASSWORD = "demo1234";

export const DEMO_ACCOUNTS: ReadonlyArray<{ email: string; role: string }> = [
  { email: "admin@demo.local", role: "Admin" },
  { email: "reviewer@demo.local", role: "Reviewer" },
  { email: "manager@demo.local", role: "Hiring manager" },
];

export function DemoCredentials({ onPick }: { onPick: (email: string, password: string) => void }) {
  const { data } = useQuery({
    queryKey: ["public-config"],
    queryFn: () => client.getPublicConfig(),
    staleTime: Infinity,
    retry: false,
  });
  if (!data?.demo) return null;

  return (
    <section className="demo-credentials" aria-labelledby="demo-credentials-title">
      <h2 id="demo-credentials-title">Demo accounts</h2>
      <p>
        Every account uses the password <code>{DEMO_PASSWORD}</code>.
      </p>
      <ul>
        {DEMO_ACCOUNTS.map((account) => (
          <li key={account.email}>
            <code>{account.email}</code>
            <button type="button" onClick={() => onPick(account.email, DEMO_PASSWORD)}>
              Use {account.role}
            </button>
          </li>
        ))}
      </ul>
    </section>
  );
}
