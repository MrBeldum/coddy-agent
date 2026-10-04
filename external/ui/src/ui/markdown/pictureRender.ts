import { LIGHT_THEMES, type UiThemeMode } from "../theme/themeCookie";
import { readAppliedUiTheme } from "../theme/uiTheme";
import { loadMermaid } from "./renderers";

/** A picture ready for an <img>: the SVG text and its drawn size in CSS pixels. */
export type RenderedPicture = { svg: string; width: number; height: number };

export type PictureKind = "mermaid" | "svg";

/** Fence labels drawn as pictures, by the first word of the info string. */
export function pictureKindOf(className: string | undefined): PictureKind | null {
  const m = /(?:^|\s)language-([\w-]+)/.exec(className || "");
  const lang = (m?.[1] || "").toLowerCase();
  if (lang === "mermaid") return "mermaid";
  if (lang === "svg") return "svg";
  return null;
}

/** An <img> source for SVG text. Inside <img> an SVG runs no script and loads nothing. */
export function svgDataUrl(svg: string): string {
  return `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`;
}

const SVG_NS = "http://www.w3.org/2000/svg";

/** A bigger picture is not something a reply should carry inline. */
const MAX_SVG_CHARS = 512 * 1024;

/**
 * Gives an SVG document what it needs to be drawn on its own, in an <img> or
 * as a downloaded file: the SVG namespace, and a width and height in pixels
 * (Mermaid writes width="100%" and a max-width style, which an <img> cannot
 * size). Throws on text that is not an SVG document.
 */
export function standaloneSvg(text: string): RenderedPicture {
  if (text.length > MAX_SVG_CHARS) throw new Error("the SVG is larger than 512 KB");
  const doc = new DOMParser().parseFromString(text, "image/svg+xml");
  const root = doc.documentElement;
  if (!root || root.nodeName.toLowerCase() !== "svg" || doc.getElementsByTagName("parsererror").length) {
    throw new Error("not an SVG document");
  }
  if (!root.getAttribute("xmlns")) root.setAttribute("xmlns", SVG_NS);
  const box = (root.getAttribute("viewBox") || "").trim().split(/[\s,]+/).map(Number);
  const vbW = box.length === 4 && box[2]! > 0 ? box[2]! : 0;
  const vbH = box.length === 4 && box[3]! > 0 ? box[3]! : 0;
  let width = pixels(root.getAttribute("width"));
  let height = pixels(root.getAttribute("height"));
  if (!width && !height && vbW && vbH) {
    width = vbW;
    height = vbH;
  } else if (width && !height && vbW && vbH) {
    height = (width * vbH) / vbW;
  } else if (height && !width && vbW && vbH) {
    width = (height * vbW) / vbH;
  }
  if (!width || !height) {
    width ||= 300;
    height ||= 150;
  }
  root.setAttribute("width", String(round(width)));
  root.setAttribute("height", String(round(height)));
  const style = root.getAttribute("style");
  if (style) {
    const kept = style
      .split(";")
      .filter((d) => d.trim() && !/^\s*max-(width|height)\s*:/i.test(d))
      .join(";");
    if (kept) root.setAttribute("style", kept);
    else root.removeAttribute("style");
  }
  return { svg: new XMLSerializer().serializeToString(root), width: round(width), height: round(height) };
}

function pixels(value: string | null): number {
  if (!value) return 0;
  const m = /^\s*([\d.]+)\s*(px)?\s*$/i.exec(value);
  return m ? Number(m[1]) || 0 : 0;
}

function round(n: number): number {
  return Math.round(n * 100) / 100;
}

/** The colours a diagram takes from the active appearance. */
export type DiagramPalette = {
  dark: boolean;
  background: string;
  node: string;
  border: string;
  text: string;
  line: string;
  secondary: string;
  tertiary: string;
  font: string;
};

/**
 * Reads the active theme's tokens. Mermaid computes shades from these, so each
 * is passed through the browser's own colour parser and dropped when it is not
 * a plain colour (a color-mix() Mermaid cannot read); Mermaid then falls back
 * to its own dark or light defaults.
 */
export function diagramPalette(theme: UiThemeMode): DiagramPalette {
  const css = getComputedStyle(document.documentElement);
  const read = (name: string) => plainColor(css.getPropertyValue(name).trim());
  return {
    dark: !LIGHT_THEMES.has(theme),
    background: read("--coddy-surface-inset"),
    node: read("--coddy-surface-raised"),
    border: read("--accent"),
    text: read("--text"),
    line: read("--muted"),
    secondary: read("--coddy-surface-field"),
    tertiary: read("--coddy-surface-sunken"),
    font: getComputedStyle(document.body).fontFamily || "sans-serif",
  };
}

const HEX = /^#(?:[0-9a-f]{3}|[0-9a-f]{6})$/i;
const RGB = /^rgba?\(\s*\d+(?:\.\d+)?\s*,\s*\d+(?:\.\d+)?\s*,\s*\d+(?:\.\d+)?\s*(?:,\s*[\d.]+\s*)?\)$/i;

function plainColor(value: string): string {
  return HEX.test(value) || RGB.test(value) ? value : "";
}

function mermaidConfig(p: DiagramPalette) {
  const vars: Record<string, string | boolean> = { darkMode: p.dark, fontFamily: p.font, fontSize: "14px" };
  const set = (key: string, value: string) => {
    if (value) vars[key] = value;
  };
  set("background", p.background);
  set("primaryColor", p.node);
  set("mainBkg", p.node);
  set("primaryBorderColor", p.border);
  set("nodeBorder", p.border);
  set("primaryTextColor", p.text);
  set("textColor", p.text);
  set("titleColor", p.text);
  set("lineColor", p.line);
  set("secondaryColor", p.secondary);
  set("tertiaryColor", p.tertiary);
  return {
    startOnLoad: false,
    securityLevel: "strict" as const,
    // Mermaid's own defaults, spelled out: a reply cannot ask for more.
    maxTextSize: 50000,
    maxEdges: 500,
    // SVG text, not HTML in a foreignObject: the picture is drawn by an <img>
    // and downloaded as a file, and both read plain SVG best.
    htmlLabels: false,
    flowchart: { htmlLabels: false },
    theme: "base" as const,
    themeVariables: vars,
  };
}

/** Recent pictures by theme and source, so a row scrolled back into view does not run Mermaid again. */
const CACHE_LIMIT = 64;
const cache = new Map<string, RenderedPicture>();

export function cachedPicture(kind: PictureKind, source: string, theme: string): RenderedPicture | undefined {
  const key = cacheKey(kind, source, theme);
  const hit = cache.get(key);
  if (hit) {
    cache.delete(key);
    cache.set(key, hit);
  }
  return hit;
}

function remember(key: string, picture: RenderedPicture) {
  cache.set(key, picture);
  while (cache.size > CACHE_LIMIT) cache.delete(cache.keys().next().value!);
}

function cacheKey(kind: PictureKind, source: string, theme: string): string {
  // An SVG block draws the same in every theme.
  return `${kind}\u0000${kind === "svg" ? "" : theme}\u0000${source}`;
}

/** Mermaid's configuration is global: one diagram at a time. */
let queue: Promise<unknown> = Promise.resolve();
let renderSeq = 0;

export function renderPicture(kind: PictureKind, source: string, theme: UiThemeMode): Promise<RenderedPicture> {
  const key = cacheKey(kind, source, theme);
  const hit = cache.get(key);
  if (hit) return Promise.resolve(hit);
  if (kind === "svg") {
    try {
      const picture = standaloneSvg(source);
      remember(key, picture);
      return Promise.resolve(picture);
    } catch (err) {
      return Promise.reject(err);
    }
  }
  const job = queue.then(async () => {
    const again = cache.get(key);
    if (again) return again;
    const mermaid = await loadMermaid();
    // The palette is read from the page when the job runs, not when it was
    // queued: a job for a theme the page has already left would draw in the
    // new colours and file the picture under the old theme.
    if (readAppliedUiTheme() !== theme) throw new Error("the theme changed before the diagram was drawn");
    mermaid.initialize(mermaidConfig(diagramPalette(theme)));
    const id = `coddy-mermaid-${++renderSeq}`;
    try {
      // parse() names the error without leaving Mermaid's error picture behind.
      await mermaid.parse(source);
      const { svg } = await mermaid.render(id, source);
      const picture = standaloneSvg(svg);
      if (readAppliedUiTheme() === theme) remember(key, picture);
      return picture;
    } finally {
      // render() works in a temporary element it does not always remove on failure.
      document.getElementById(id)?.remove();
      document.getElementById(`d${id}`)?.remove();
    }
  });
  queue = job.catch(() => undefined);
  return job;
}

/** For tests: forget every rendered picture. */
export function clearPictureCache() {
  cache.clear();
}
