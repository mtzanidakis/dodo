// Builds the embedded frontend assets into internal/web/dist so they are
// served from /static/{version}/... (no CDN):
//   - vendor.js   : htmx + alpine, concatenated
//   - app.css     : copied from web/src/app.css
//   - app.js      : copied from web/src/app.js
//   - sw.js       : copied from web/src/sw.js (served at /sw.js)
//   - *.png       : logo and installed-app icons
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

// dateformat.js ships as its own classic script so the test suite can import
// exactly the file the browser runs. app.js reads the global it publishes, so
// it must be loaded first (see templates/layout.html).
const dateFormatJS = fs.readFileSync(path.join(srcDir, "dateformat.js"), "utf8");
fs.writeFileSync(path.join(out, "dateformat.js"), dateFormatJS);

const appJS = fs.readFileSync(path.join(srcDir, "app.js"), "utf8");
fs.writeFileSync(path.join(out, "app.js"), appJS);

// sw.js is served from the site root (/sw.js), not /static/{version}/, so a
// worker can control the whole app; see internal/web/pwa.go.
const swJS = fs.readFileSync(path.join(srcDir, "sw.js"), "utf8");
fs.writeFileSync(path.join(out, "sw.js"), swJS);

const images = ["logo.png", "icon-192.png", "icon-512.png", "icon-maskable-512.png", "apple-touch-icon.png"];
const hasher = crypto.createHash("sha256").update(vendorJS).update(appCSS).update(dateFormatJS).update(appJS).update(swJS);
for (const name of images) {
  const b = fs.readFileSync(path.join(srcDir, name));
  fs.writeFileSync(path.join(out, name), b);
  hasher.update(b);
}

const hash = hasher.digest("hex").slice(0, 12);
fs.writeFileSync(path.join(out, "version.txt"), hash + "\n");

console.log("dist built:", "version=" + hash, "(vendor.js, app.css, dateformat.js, app.js, sw.js, " + images.join(", ") + ", version.txt)");
