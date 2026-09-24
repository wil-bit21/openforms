import { afterEach, describe, expect, it, vi } from "vitest";
import { handleMessage, init, MOUNTED_ATTR, mount, originFromScript, RESIZE, SUBMITTED } from "./embed.js";

const ORIGIN = "https://forms.example.com";

afterEach(() => {
  document.body.innerHTML = "";
});

function container(slug = "job-application") {
  const div = document.createElement("div");
  div.setAttribute("data-openforms", slug);
  document.body.appendChild(div);
  return div;
}

function message(data: unknown, origin: string, source: Window | null) {
  return new MessageEvent("message", { data, origin, source });
}

describe("originFromScript", () => {
  it("derives the origin from the script src", () => {
    expect(originFromScript("https://forms.example.com/embed.js?v=1", "https://site.test")).toBe(ORIGIN);
  });
  it("resolves relative src against the fallback and tolerates garbage", () => {
    expect(originFromScript("/embed.js", "https://site.test")).toBe("https://site.test");
    expect(originFromScript(null, "https://site.test")).toBe("https://site.test");
    expect(originFromScript("http://[bad", "https://site.test")).toBe("https://site.test");
  });
});

describe("mount", () => {
  it("injects a full-width iframe pointing at the hosted embed page", () => {
    const div = container("job application");
    const iframe = mount(div, ORIGIN)!;
    expect(iframe.src).toBe(`${ORIGIN}/f/job%20application?embed=1`);
    expect(iframe.title).toBe("Form: job application");
    expect(iframe.style.width).toBe("100%");
    expect(div.hasAttribute(MOUNTED_ATTR)).toBe(true);
  });

  it("is idempotent and ignores empty slugs", () => {
    const div = container();
    expect(mount(div, ORIGIN)).not.toBeNull();
    expect(mount(div, ORIGIN)).toBeNull();
    expect(div.querySelectorAll("iframe")).toHaveLength(1);
    expect(mount(container("  "), ORIGIN)).toBeNull();
  });

  it("uses data-openforms-title when given", () => {
    const div = container();
    div.setAttribute("data-openforms-title", "Apply now");
    expect(mount(div, ORIGIN)!.title).toBe("Apply now");
  });
});

describe("handleMessage", () => {
  it("resizes the matching iframe for messages from the openforms origin", () => {
    const iframe = mount(container(), ORIGIN)!;
    handleMessage(message({ type: RESIZE, height: 812.4 }, ORIGIN, iframe.contentWindow), ORIGIN, document);
    expect(iframe.style.height).toBe("813px");
  });

  it("ignores messages from other origins", () => {
    const iframe = mount(container(), ORIGIN)!;
    const before = iframe.style.height;
    handleMessage(message({ type: RESIZE, height: 999 }, "https://evil.example", iframe.contentWindow), ORIGIN, document);
    expect(iframe.style.height).toBe(before);
  });

  it("ignores messages whose source is not one of our iframes", () => {
    const iframe = mount(container(), ORIGIN)!;
    const before = iframe.style.height;
    handleMessage(message({ type: RESIZE, height: 999 }, ORIGIN, window), ORIGIN, document);
    expect(iframe.style.height).toBe(before);
  });

  it("ignores malformed or absurd heights and non-object payloads", () => {
    const iframe = mount(container(), ORIGIN)!;
    const before = iframe.style.height;
    for (const height of ["900", -5, 0, Number.NaN, 1e9]) {
      handleMessage(message({ type: RESIZE, height }, ORIGIN, iframe.contentWindow), ORIGIN, document);
    }
    handleMessage(message("openforms:resize", ORIGIN, iframe.contentWindow), ORIGIN, document);
    handleMessage(message(null, ORIGIN, iframe.contentWindow), ORIGIN, document);
    expect(iframe.style.height).toBe(before);
  });

  it("re-dispatches submitted as a bubbling CustomEvent on the container", () => {
    const div = container();
    const iframe = mount(div, ORIGIN)!;
    const listener = vi.fn();
    document.body.addEventListener(SUBMITTED, listener);
    handleMessage(message({ type: SUBMITTED, id: "sub-1", state: "new" }, ORIGIN, iframe.contentWindow), ORIGIN, document);
    expect(listener).toHaveBeenCalledTimes(1);
    const event = listener.mock.calls[0]![0] as CustomEvent;
    expect(event.target).toBe(div);
    expect(event.detail).toEqual({ id: "sub-1", state: "new" });
  });
});

describe("init", () => {
  it("mounts every container and wires the message listener", () => {
    const a = container("contact");
    const b = container("job-application");
    const api = init(document, window, `${ORIGIN}/embed.js`);
    expect(api.origin).toBe(ORIGIN);
    const frameA = a.querySelector("iframe")!;
    expect(b.querySelector("iframe")).not.toBeNull();

    window.dispatchEvent(message({ type: RESIZE, height: 640 }, ORIGIN, frameA.contentWindow));
    expect(frameA.style.height).toBe("640px");

    const c = container("late");
    api.scan();
    expect(c.querySelector("iframe")).not.toBeNull();

    api.destroy();
    window.dispatchEvent(message({ type: RESIZE, height: 700 }, ORIGIN, frameA.contentWindow));
    expect(frameA.style.height).toBe("640px");
  });
});
