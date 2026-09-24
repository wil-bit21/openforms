import { createServer } from "node:http";
import { SINK_PORT } from "./env";

export interface Delivery {
  path: string;
  headers: Record<string, string>;
  body: string;
  receivedAt: string;
}

/** Records every POST; GET /received returns the recorded deliveries (tests run in other processes). */
export function startWebhookSink(port = SINK_PORT): Promise<{ close(): Promise<void> }> {
  const deliveries: Delivery[] = [];
  const server = createServer((req, res) => {
    if (req.method === "GET" && req.url === "/received") {
      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(JSON.stringify(deliveries));
      return;
    }
    if (req.method === "POST") {
      const chunks: Buffer[] = [];
      req.on("data", (c: Buffer) => chunks.push(c));
      req.on("end", () => {
        const headers: Record<string, string> = {};
        for (const [k, v] of Object.entries(req.headers)) headers[k] = Array.isArray(v) ? v.join(",") : (v ?? "");
        deliveries.push({
          path: req.url ?? "",
          headers,
          body: Buffer.concat(chunks).toString("utf8"),
          receivedAt: new Date().toISOString(),
        });
        res.writeHead(204);
        res.end();
      });
      return;
    }
    res.writeHead(404);
    res.end();
  });
  return new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(port, "0.0.0.0", () =>
      resolve({ close: () => new Promise<void>((r) => server.close(() => r())) }),
    );
  });
}

export async function receivedDeliveries(): Promise<Delivery[]> {
  const res = await fetch(`http://localhost:${SINK_PORT}/received`);
  return (await res.json()) as Delivery[];
}
