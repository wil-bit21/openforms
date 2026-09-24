import { MAILPIT_URL } from "./env";

export interface MailpitMessage {
  ID: string;
  Subject: string;
  To: Array<{ Address: string; Name: string }>;
}

export async function listMessages(): Promise<MailpitMessage[]> {
  const res = await fetch(`${MAILPIT_URL}/api/v1/messages?limit=500`);
  if (!res.ok) throw new Error(`mailpit: ${res.status}`);
  const body = (await res.json()) as { messages?: MailpitMessage[] };
  return body.messages ?? [];
}

export async function clearMessages(): Promise<void> {
  await fetch(`${MAILPIT_URL}/api/v1/messages`, { method: "DELETE" });
}
