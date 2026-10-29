import { describe, it, expect, beforeEach } from "vitest";

import "./dateformat.js";
import "./app.js";

function metas() {
  return [...document.querySelectorAll('meta[name="theme-color"]')].map((m) => m.getAttribute("content"));
}

beforeEach(() => {
  document.head.innerHTML = `
    <meta name="theme-color" content="#ffffff" media="(prefers-color-scheme: light)">
    <meta name="theme-color" content="#16191d" media="(prefers-color-scheme: dark)">`;
});

describe("theme-color", () => {
  it("follows an explicit theme regardless of the OS scheme", () => {
    window.dodoSetTheme("dark");
    expect(metas()).toEqual(["#16191d", "#16191d"]);
    window.dodoSetTheme("light");
    expect(metas()).toEqual(["#ffffff", "#ffffff"]);
  });

  it("goes back to per-scheme colors for the system theme", () => {
    window.dodoSetTheme("dark");
    window.dodoSetTheme("system");
    expect(metas()).toEqual(["#ffffff", "#16191d"]);
  });
});
