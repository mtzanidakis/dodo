// Copies/symlinks app.js into internal/web/dist. Kept minimal: the
// real frontend build (tailwind + htmx + alpine) is wired here.
const fs = require("fs");
const path = require("path");
const out = path.join("..", "internal", "web", "dist");
fs.mkdirSync(out, { recursive: true });
if (fs.existsSync("src/app.js")) {
  fs.copyFileSync("src/app.js", path.join(out, "app.js"));
}
