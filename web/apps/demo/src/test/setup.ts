import "@testing-library/jest-dom/vitest";
import { afterAll, afterEach, beforeAll, beforeEach } from "vitest";
import { cleanup } from "@testing-library/react";
import { server, resetDemoApi } from "./server";

class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
(globalThis as unknown as { ResizeObserver: unknown }).ResizeObserver ??= ResizeObserverStub;

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
beforeEach(() => resetDemoApi());
afterEach(() => {
  cleanup();
  server.resetHandlers();
});
afterAll(() => server.close());
