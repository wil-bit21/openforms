import { BASE_URL, readState } from "./env";

export interface ApiResponse<T> {
  status: number;
  body: T;
}

export async function api<T = unknown>(
  method: string,
  path: string,
  body?: unknown,
  apiKey: string | null = readState().apiKey,
): Promise<ApiResponse<T>> {
  const headers: Record<string, string> = {};
  if (body !== undefined) headers["Content-Type"] = "application/json";
  if (apiKey) headers.Authorization = `Bearer ${apiKey}`;
  const res = await fetch(BASE_URL + path, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const text = await res.text();
  return { status: res.status, body: (text ? JSON.parse(text) : null) as T };
}

export async function createPublicSubmission(
  slug: string,
  data: Record<string, unknown>,
): Promise<{ id: string; receiptToken: string }> {
  const res = await api<{ id: string; receiptToken: string }>(
    "POST",
    `/api/v1/public/forms/${slug}/submissions`,
    { data },
    null,
  );
  if (res.status !== 201) throw new Error(`submit ${slug}: ${res.status} ${JSON.stringify(res.body)}`);
  return res.body;
}
