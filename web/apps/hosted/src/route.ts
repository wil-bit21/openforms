export type Route =
  | { kind: "form"; slug: string; embed: boolean }
  | { kind: "status"; id: string; token: string; embed: boolean }
  | { kind: "notFound"; embed: boolean };

function decode(segment: string): string {
  try {
    return decodeURIComponent(segment);
  } catch {
    return segment;
  }
}

export function parseRoute(pathname: string, search: string): Route {
  const params = new URLSearchParams(search);
  const embed = params.get("embed") === "1";
  // In `vite dev` the app is served under its base path; in production the Go server serves it at /f and /s.
  const path = pathname.replace(/^\/_app\/hosted(?=\/)/, "");
  const segments = path.split("/").filter(Boolean).map(decode);

  if (segments.length === 2 && segments[0] === "f") {
    return { kind: "form", slug: segments[1]!, embed };
  }
  if (segments.length === 2 && segments[0] === "s") {
    const token = params.get("token");
    if (token) return { kind: "status", id: segments[1]!, token, embed };
  }
  return { kind: "notFound", embed };
}

export function statusHref(id: string, token: string): string {
  return `/s/${encodeURIComponent(id)}?token=${encodeURIComponent(token)}`;
}
