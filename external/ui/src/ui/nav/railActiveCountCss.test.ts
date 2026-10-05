/**
 * Contract: the active-turn badge of History and Scheduler sits on the icon on
 * both rails. On the icon-only rail it is placed from the corner of the 44px
 * hit, which the icon is centred in; on the labelled rail the same offset is
 * measured from the 44px icon track, never from the corner of the whole pill,
 * where it read as belonging to the label.
 */
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { expect, test } from "vitest";

const dir = dirname(fileURLToPath(import.meta.url));
const css = readFileSync(join(dir, "../../styles.css"), "utf8");

function rule(selector: string): string {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  return (
    new RegExp(`(^|\\n)${escaped}\\s*\\{[^}]*\\}`, "s").exec(css)?.[0] ?? ""
  );
}

function px(block: string, prop: string): number {
  const m = new RegExp(`(^|[\\s{;])${prop}:\\s*(-?[\\d.]+)px`).exec(block);
  return m ? Number(m[2]) : Number.NaN;
}

test("the badge keeps the icon-only offset from the icon on the labelled rail", () => {
  const narrow = rule(".rail-active-count");
  const wide = rule(".rail-nav-hit-wide .rail-active-count");
  expect(wide).not.toBe("");

  // The icon-only hit is 44px with a 1px border, so its padding box is 42px
  // and the icon's centre is 21px from either edge of it.
  const iconCentre = 21;
  const rightOfCentre = 42 - px(narrow, "right") - iconCentre;
  const aboveCentre = iconCentre - px(narrow, "top");

  // The labelled hit centres the icon in its first 44px track (22px in) and
  // the icon in the hit's height, so the badge is placed from those.
  expect(px(wide, "left") - 22).toBe(rightOfCentre);
  expect(wide).toMatch(/transform:\s*translateX\(-100%\)/);
  expect(wide).toMatch(new RegExp(`top:\\s*calc\\(50% - ${aboveCentre}px\\)`));
  expect(wide).toMatch(/right:\s*auto/);
});
