import "katex/dist/katex.min.css";
import { useCallback, useEffect, useRef, useState, type KeyboardEvent } from "react";
import { useT } from "../i18n/I18nProvider";
import { CodeBlockCopyButton } from "../messages/CodeBlockCopyButton";
import { FigureViewToggle } from "./DiagramBlock";
import { loadKatex, type KatexApi } from "./renderers";

/** How every formula is typeset. */
export const KATEX_OPTIONS = {
  // Bad TeX comes out as KaTeX's own red source rather than an exception.
  throwOnError: false,
  // No \href, \url, \includegraphics or \htmlClass from a model's answer.
  trust: false,
  strict: "ignore" as const,
  output: "htmlAndMathml" as const,
  // Bounds on what one formula may cost: sizes in em, macro expansions.
  maxSize: 20,
  maxExpand: 1000,
};

/** Formulas already typeset, by mode and source: a row scrolled back into view is not typeset again. */
const CACHE_LIMIT = 500;
const cache = new Map<string, string>();
let katex: KatexApi | undefined;

function typeset(api: KatexApi, source: string, display: boolean): string {
  const key = `${display ? "D" : "I"}\u0000${source}`;
  const hit = cache.get(key);
  if (hit !== undefined) return hit;
  const html = api.renderToString(source, { ...KATEX_OPTIONS, displayMode: display });
  cache.set(key, html);
  while (cache.size > CACHE_LIMIT) cache.delete(cache.keys().next().value!);
  return html;
}

/** For tests: forget the loaded KaTeX and every typeset formula. */
export function resetMathForTests() {
  katex = undefined;
  cache.clear();
}

/** The typeset HTML of a formula, or null while KaTeX is still loading (or failed to). */
function useTypeset(source: string, display: boolean): string | null {
  const [api, setApi] = useState<KatexApi | undefined>(katex);
  useEffect(() => {
    if (api) return;
    let live = true;
    loadKatex().then(
      (loaded) => {
        katex = loaded;
        if (live) setApi(() => loaded);
      },
      () => {
        // Offline or a stale chunk: the source stays on screen as code.
      },
    );
    return () => {
      live = false;
    };
  }, [api]);
  if (!api) return null;
  try {
    return typeset(api, source, display);
  } catch {
    return null;
  }
}

function copyText(text: string): Promise<void> {
  return navigator.clipboard.writeText(text);
}

/** `$...$` in a sentence: typeset in place; the source is its tooltip and a click copies it. */
export function MathInline(props: { source: string }) {
  const { t } = useT();
  const html = useTypeset(props.source, false);
  const [copied, setCopied] = useState(false);
  const timer = useRef<number | undefined>(undefined);
  useEffect(() => () => window.clearTimeout(timer.current), []);
  const delimited = `$${props.source}$`;

  const onCopy = useCallback(() => {
    copyText(delimited).then(
      () => {
        setCopied(true);
        window.clearTimeout(timer.current);
        timer.current = window.setTimeout(() => setCopied(false), 900);
      },
      () => setCopied(false),
    );
  }, [delimited]);

  const onKeyDown = useCallback(
    (e: KeyboardEvent<HTMLElement>) => {
      if (e.key === "Enter" || e.key === " ") {
        e.preventDefault();
        onCopy();
      }
    },
    [onCopy],
  );

  const title = copied ? t("messages.copied") : t("markdown.math.inlineTitle", { source: delimited });
  if (html === null) {
    return (
      <code className="md-math-inline md-math-pending" data-testid="md-math-inline" title={title}>
        {delimited}
      </code>
    );
  }
  return (
    <span
      className={copied ? "md-math-inline is-copied" : "md-math-inline"}
      role="button"
      tabIndex={0}
      title={title}
      aria-label={t("markdown.math.copySource")}
      data-testid="md-math-inline"
      data-source={props.source}
      onClick={onCopy}
      onKeyDown={onKeyDown}
      dangerouslySetInnerHTML={{ __html: html }}
    />
  );
}

/** `$$...$$` or a ```math fence: a typeset block with a switch to its source and a copy button. */
export function MathBlock(props: { source: string }) {
  const { t } = useT();
  const html = useTypeset(props.source, true);
  const [view, setView] = useState<"picture" | "code">("picture");
  const showing = html === null ? "code" : view;
  return (
    <figure className="md-figure md-figure--math" data-testid="md-math-block" data-view={showing}>
      <div className="md-figure-head">
        <span className="md-figure-label">{t("markdown.math.label")}</span>
        <div className="md-figure-actions">
          <FigureViewToggle
            showing={showing}
            pictureLabel={t("markdown.math.showFormula")}
            pictureDisabled={html === null}
            onChange={setView}
          />
          <CodeBlockCopyButton textToCopy={props.source} dataTestId="md-math-copy" />
        </div>
      </div>
      {showing === "picture" && html !== null ? (
        <div className="md-math-display" dangerouslySetInnerHTML={{ __html: html }} />
      ) : (
        <pre className="md-figure-code">
          <code>{props.source}</code>
        </pre>
      )}
    </figure>
  );
}
