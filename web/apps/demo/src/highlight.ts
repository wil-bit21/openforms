import { codeToHtml } from "shiki";

export type Lang = "yaml" | "bash" | "html";

/** Dual-theme highlighting; colours come from --shiki-light / --shiki-dark CSS variables. */
export function highlight(code: string, lang: Lang): Promise<string> {
  return codeToHtml(code, {
    lang,
    themes: { light: "github-light", dark: "github-dark" },
    defaultColor: false,
  });
}
