import { describe, it, expect, beforeEach, afterEach } from "vitest";

// Both files are classic scripts; importing them runs them against jsdom's
// document exactly as the browser would, including the listeners app.js
// attaches on load.
import "./dateformat.js";
import "./app.js";

const PATTERN = "DD/MM/YYYY HH:mm";

function click(el) {
  el.dispatchEvent(new window.MouseEvent("click", { bubbles: true }));
}

function press(key) {
  const ev = new window.KeyboardEvent("keydown", { key, bubbles: true, cancelable: true });
  document.dispatchEvent(ev);
  return ev;
}

function typeInto(input, value) {
  input.value = value;
  input.dispatchEvent(new window.Event("input", { bubbles: true }));
}

function popup() {
  return document.querySelector(".dp");
}

function dueInput() {
  return document.querySelector("input[name=due_at]");
}

function setup({ pattern = PATTERN, value = "" } = {}) {
  document.body.innerHTML = `
    <form id="f">
      <input type="text" name="due_at" value="${value}"
             data-datepicker="${pattern}"
             data-dow="Mon,Tue,Wed,Thu,Fri,Sat,Sun"
             data-dp-today="Today" data-dp-done="OK">
    </form>`;
  return dueInput();
}

beforeEach(() => {
  document.body.innerHTML = "";
});

afterEach(() => {
  // The picker keeps one module-level open state; make sure a test never
  // leaves it open for the next one.
  press("Escape");
  document.body.innerHTML = "";
});

describe("attaching", () => {
  it("opens on click and renders a month grid", () => {
    const input = setup({ value: "05/07/2026 09:30" });
    expect(popup()).toBeNull();

    click(input);
    const p = popup();
    expect(p).not.toBeNull();
    expect(p.querySelector("[data-dp-month]").textContent).toBe("July 2026");
    // 7 weekday headings, in the order the server sent them.
    const dow = [...p.querySelectorAll(".dp-dow")].map((el) => el.textContent);
    expect(dow).toEqual(["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"]);
    expect(p.querySelectorAll(".dp-day")).toHaveLength(31);
    expect(p.querySelector(".dp-day.selected").textContent).toBe("5");
  });

  it("leaves an input without a pattern alone", () => {
    const input = setup({ pattern: "" });
    click(input);
    expect(popup()).toBeNull();
  });

  it("uses the localized labels the server sent", () => {
    document.body.innerHTML = `
      <input type="text" name="due_at" data-datepicker="${PATTERN}"
             data-dow="ΔΕΥ,ΤΡΙ,ΤΕΤ,ΠΕΜ,ΠΑΡ,ΣΑΒ,ΚΥΡ"
             data-dp-today="Σήμερα" data-dp-done="OK">`;
    click(dueInput());
    const p = popup();
    expect([...p.querySelectorAll(".dp-dow")].map((el) => el.textContent)[0]).toBe("ΔΕΥ");
    expect(p.querySelector("[data-dp-todaybtn]").textContent).toBe("Σήμερα");
  });

  it("never puts a submitting button inside the form's reach", () => {
    click(setup());
    for (const b of popup().querySelectorAll("button")) {
      expect(b.type).toBe("button");
    }
  });
});

describe("selecting", () => {
  it("writes the chosen day back in the user's pattern", () => {
    const input = setup({ value: "05/07/2026 09:30" });
    click(input);
    const day14 = [...popup().querySelectorAll(".dp-day")].find((b) => b.textContent === "14");
    click(day14);
    expect(input.value).toBe("14/07/2026 09:30");
  });

  it("moves months without touching the field", () => {
    const input = setup({ value: "05/07/2026 09:30" });
    click(input);
    click(popup().querySelector("[data-dp-next]"));
    expect(popup().querySelector("[data-dp-month]").textContent).toBe("August 2026");
    expect(input.value).toBe("05/07/2026 09:30");

    click(popup().querySelector("[data-dp-prev]"));
    expect(popup().querySelector("[data-dp-month]").textContent).toBe("July 2026");
  });

  it("commits and closes on OK", () => {
    const input = setup({ value: "05/07/2026 09:30" });
    click(input);
    click(popup().querySelector("[data-dp-done]"));
    expect(popup()).toBeNull();
    expect(input.value).toBe("05/07/2026 09:30");
  });

  it("closes on a click outside", () => {
    click(setup({ value: "05/07/2026 09:30" }));
    expect(popup()).not.toBeNull();
    click(document.body);
    expect(popup()).toBeNull();
  });
});

describe("typing", () => {
  it("follows the typed date into another month", () => {
    const input = setup({ value: "05/07/2026 09:30" });
    click(input);
    typeInto(input, "03/09/2026 07:15");
    expect(popup().querySelector("[data-dp-month]").textContent).toBe("September 2026");
    expect(popup().querySelector(".dp-day.selected").textContent).toBe("3");
  });

  // Regression: the popup seeded its time field when it opened, so confirming
  // the calendar overwrote a time typed afterwards.
  it("keeps a time typed after the popup opened", () => {
    const input = setup({ value: "05/07/2026 09:30" });
    click(input);
    typeInto(input, "05/07/2026 18:45");
    expect(popup().querySelector("[data-dp-time]").value).toBe("18:45");

    click(popup().querySelector("[data-dp-done]"));
    expect(input.value).toBe("05/07/2026 18:45");
  });

  it("ignores half-typed input instead of jumping around", () => {
    const input = setup({ value: "05/07/2026 09:30" });
    click(input);
    typeInto(input, "05/0");
    // Still showing what was there before the user started editing.
    expect(popup().querySelector("[data-dp-month]").textContent).toBe("July 2026");
  });
});

describe("keyboard", () => {
  it("moves by day and week", () => {
    const input = setup({ value: "05/07/2026 09:30" });
    click(input);
    press("ArrowRight");
    expect(popup().querySelector(".dp-day.selected").textContent).toBe("6");
    press("ArrowDown");
    expect(popup().querySelector(".dp-day.selected").textContent).toBe("13");
    press("ArrowUp");
    press("ArrowLeft");
    expect(popup().querySelector(".dp-day.selected").textContent).toBe("5");
  });

  it("crosses a month boundary", () => {
    const input = setup({ value: "31/07/2026 09:30" });
    click(input);
    press("ArrowRight");
    expect(popup().querySelector("[data-dp-month]").textContent).toBe("August 2026");
    expect(popup().querySelector(".dp-day.selected").textContent).toBe("1");
  });

  it("picks on Enter and suppresses the form submit", () => {
    const input = setup({ value: "05/07/2026 09:30" });
    click(input);
    press("ArrowRight");
    const ev = press("Enter");
    expect(ev.defaultPrevented).toBe(true);
    expect(input.value).toBe("06/07/2026 09:30");
    expect(popup()).toBeNull();
  });

  it("closes on Escape without changing the field", () => {
    const input = setup({ value: "05/07/2026 09:30" });
    click(input);
    press("ArrowRight");
    press("Escape");
    expect(popup()).toBeNull();
    expect(input.value).toBe("05/07/2026 09:30");
  });
});
