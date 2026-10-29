import { describe, it, expect, beforeEach, afterEach } from "vitest";

import "./dateformat.js";
import "./app.js";

function metas() {
  return [...document.querySelectorAll('meta[name="theme-color"]')].map((m) => m.getAttribute("content"));
}

function manifest() {
  return document.querySelector('link[rel="manifest"]').getAttribute("href");
}

function osPrefersDark(dark) {
  window.matchMedia = (q) => ({ matches: dark && q.includes("dark") });
}

beforeEach(() => {
  document.head.innerHTML = `
    <meta name="theme-color" content="#ffffff" media="(prefers-color-scheme: light)">
    <meta name="theme-color" content="#16191d" media="(prefers-color-scheme: dark)">
    <link rel="manifest" href="/manifest.webmanifest">`;
  osPrefersDark(false);
});

afterEach(() => {
  delete window.matchMedia;
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

describe("manifest", () => {
  it("uses the dark manifest for the dark theme", () => {
    window.dodoSetTheme("dark");
    expect(manifest()).toBe("/manifest.webmanifest?theme=dark");
    window.dodoSetTheme("light");
    expect(manifest()).toBe("/manifest.webmanifest");
  });

  it("follows the OS scheme for the system theme", () => {
    osPrefersDark(true);
    window.dodoSetTheme("system");
    expect(manifest()).toBe("/manifest.webmanifest?theme=dark");
    osPrefersDark(false);
    window.dodoSetTheme("system");
    expect(manifest()).toBe("/manifest.webmanifest");
  });

  it("keeps the light manifest for an explicit light theme on a dark OS", () => {
    osPrefersDark(true);
    window.dodoSetTheme("light");
    expect(manifest()).toBe("/manifest.webmanifest");
  });
});
