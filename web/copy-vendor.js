// Builds the embedded frontend assets into internal/web/dist so they are
// served from /static/{version}/... (no CDN):
//   - vendor.js   : htmx + alpine, concatenated
//   - app.css     : copied from web/src/app.css
//   - app.js      : copied from web/src/app.js
//   - version.txt : short content hash used for cache-busting the /static path
const fs = require("fs");
const path = require("path");
const crypto = require("crypto");

const srcDir = path.join(__dirname, "src");
const out = path.join(__dirname, "..", "internal", "web", "dist");
fs.mkdirSync(out, { recursive: true });

const parts = [];

function tryCopy(pkg, rel) {
  const p = path.join(__dirname, "node_modules", pkg, rel);
  if (fs.existsSync(p)) {
    const pkgJson = JSON.parse(
      fs.readFileSync(path.join(__dirname, "node_modules", pkg, "package.json"), "utf8"),
    );
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

const vendorJS = parts.join("\n\n");
fs.writeFileSync(path.join(out, "vendor.js"), vendorJS);

const appCSS = fs.readFileSync(path.join(srcDir, "app.css"), "utf8");
fs.writeFileSync(path.join(out, "app.css"), appCSS);

const appJS = fs.readFileSync(path.join(srcDir, "app.js"), "utf8");
fs.writeFileSync(path.join(out, "app.js"), appJS);

const logo = fs.readFileSync(path.join(srcDir, "logo.png"));
fs.writeFileSync(path.join(out, "logo.png"), logo);

const hash = crypto
  .createHash("sha256")
  .update(vendorJS)
  .update(appCSS)
  .update(appJS)
  .update(logo)
  .digest("hex")
  .slice(0, 12);
fs.writeFileSync(path.join(out, "version.txt"), hash + "\n");

console.log("dist built:", "version=" + hash, "(vendor.js, app.css, app.js, logo.png, version.txt)");
