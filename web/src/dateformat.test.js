import { describe, it, expect, beforeAll } from "vitest";

// dateformat.js is a classic script that publishes a global, exactly as the
// browser loads it. Importing it for the side effect keeps the tested code
// identical to the shipped code.
import "./dateformat.js";

let format, parse, hasTime, pad;

beforeAll(() => {
  ({ format, parse, hasTime, pad } = globalThis.dodoDateFormat);
});

// The presets offered by the server (internal/dateformat.Presets). These must
// stay in sync: the browser has to render and read back exactly what Go does.
const PRESETS = ["DD/MM/YYYY", "MM/DD/YYYY", "YYYY-MM-DD", "D MMM YYYY", "MMM D, YYYY"];

describe("format", () => {
  const ref = new Date(2026, 6, 5, 9, 4); // 5 July 2026, 09:04 local

  it.each([
    ["DD/MM/YYYY", "05/07/2026"],
    ["MM/DD/YYYY", "07/05/2026"],
    ["YYYY-MM-DD", "2026-07-05"],
    ["D MMM YYYY", "5 Jul 2026"],
    ["D MMMM YYYY", "5 July 2026"],
    ["MMM D, YYYY", "Jul 5, 2026"],
    ["D/M/YY", "5/7/26"],
    ["DD/MM/YYYY HH:mm", "05/07/2026 09:04"],
  ])("renders %s", (pattern, want) => {
    expect(format(ref, pattern)).toBe(want);
  });

  it("returns an empty string without a pattern", () => {
    expect(format(ref, "")).toBe("");
  });

  it("pads single digits", () => {
    expect(pad(4)).toBe("04");
    expect(pad(11)).toBe("11");
  });
});

describe("parse", () => {
  it.each([
    ["05/07/2026", "DD/MM/YYYY", [2026, 6, 5, 0, 0]],
    ["07/05/2026", "MM/DD/YYYY", [2026, 6, 5, 0, 0]],
    ["5/7/2026", "D/M/YYYY", [2026, 6, 5, 0, 0]],
    ["5 Jul 2026", "D MMM YYYY", [2026, 6, 5, 0, 0]],
    ["5 jul 2026", "D MMM YYYY", [2026, 6, 5, 0, 0]],
    ["5 July 2026", "D MMMM YYYY", [2026, 6, 5, 0, 0]],
    ["05.07.26", "DD.MM.YY", [2026, 6, 5, 0, 0]],
    ["05/07/2026 09:04", "DD/MM/YYYY HH:mm", [2026, 6, 5, 9, 4]],
    ["  05/07/2026  ", "DD/MM/YYYY", [2026, 6, 5, 0, 0]],
  ])("reads %s as %s", (input, pattern, [y, mo, d, h, mi]) => {
    const got = parse(input, pattern);
    expect(got).toBeInstanceOf(Date);
    expect([got.getFullYear(), got.getMonth(), got.getDate(), got.getHours(), got.getMinutes()])
      .toEqual([y, mo, d, h, mi]);
  });

  it.each([
    ["05-07-2026", "DD/MM/YYYY", "wrong separator"],
    ["05/07/2026x", "DD/MM/YYYY", "trailing junk"],
    ["x05/07/2026", "DD/MM/YYYY", "leading junk"],
    ["31/02/2026", "DD/MM/YYYY", "a day that does not exist"],
    ["13/13/2026", "DD/MM/YYYY", "month 13"],
    ["5 Smarch 2026", "D MMMM YYYY", "not a month name"],
    ["", "DD/MM/YYYY", "empty input"],
    ["05/07/2026", "", "no pattern"],
  ])("rejects %s (%s)", (input, pattern) => {
    expect(parse(input, pattern)).toBeNull();
  });

  // 31 February must not silently become 3 March: the picker would highlight a
  // day the user never typed.
  it("does not roll over out-of-range days", () => {
    expect(parse("31/04/2026", "DD/MM/YYYY")).toBeNull();
    expect(parse("29/02/2026", "DD/MM/YYYY")).toBeNull();
    expect(parse("29/02/2028", "DD/MM/YYYY")).not.toBeNull(); // 2028 is a leap year
  });
});

describe("round trip", () => {
  it.each(PRESETS)("survives %s with a time", (preset) => {
    const pattern = preset + " HH:mm";
    const want = new Date(2026, 6, 5, 9, 4);
    const got = parse(format(want, pattern), pattern);
    expect(got?.getTime()).toBe(want.getTime());
  });

  it.each(PRESETS)("survives %s without a time", (preset) => {
    const want = new Date(2026, 6, 5, 0, 0);
    const got = parse(format(want, preset), preset);
    expect(got?.getTime()).toBe(want.getTime());
  });
});

describe("hasTime", () => {
  it("detects a clock in the pattern", () => {
    expect(hasTime("DD/MM/YYYY HH:mm")).toBe(true);
    expect(hasTime("DD/MM/YYYY")).toBe(false);
    expect(hasTime("")).toBe(false);
  });
});
