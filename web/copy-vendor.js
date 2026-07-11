// Bundles htmx + alpine into internal/web/dist/vendor.js so they are
// served from /static/{version}/vendor.js (no CDN).
const fs = require("fs");
const path = require("path");

const out = path.join("..", "internal", "web", "dist");
fs.mkdirSync(out, { recursive: true });

const parts = [];

function tryCopy(pkg, rel) {
  const p = path.join("node_modules", pkg, rel);
  if (fs.existsSync(p)) {
    const pkgJson = JSON.parse(fs.readFileSync(path.join("node_modules", pkg, "package.json"), "utf8"));
    parts.push(`/* ${pkg}@${pkgJson.version} */`);
    parts.push(fs.readFileSync(p, "utf8"));
    return true;
  }
  return false;
}

let ok = true;
if (!tryCopy("htmx.org", "dist/htmx.min.js")) {
  console.error("htmx.org not found; run `npm install`");
  ok = false;
}
if (!tryCopy("alpinejs", "dist/cdn.min.js")) {
  console.error("alpinejs not found; run `npm install`");
  ok = false;
}
if (!ok) process.exit(1);

fs.writeFileSync(path.join(out, "vendor.js"), parts.join("\n\n"));
console.log("vendor.js written (" + parts.length + " chunks)");