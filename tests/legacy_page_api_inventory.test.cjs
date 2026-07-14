const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");

const root = path.resolve(__dirname, "..");
const staticRoot = path.join(root, "app", "static");
const inventoryPath = path.join(root, "docs", "migration", "legacy-page-routes.json");

function walk(directory) {
  return fs.readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const fullPath = path.join(directory, entry.name);
    if (entry.isDirectory()) {
      return entry.name === "vendor" ? [] : walk(fullPath);
    }
    return /\.(?:html|js)$/.test(entry.name) && !/\.test\.js$/.test(entry.name) ? [fullPath] : [];
  });
}

function normalizeRoute(value) {
  return value
    .replace(/\$\{params\.toString.*$/, "")
    .replace(/\$\{encodeURIComponent\([^)]*\)\}/g, ":param")
    .replace(/\?.*$/, "")
    .replace(/\$\{[^}]+\}/g, ":param")
    .replace(/\/:param(?=\/|$)/g, "/:param");
}

function discoveredRoutes() {
  const routes = new Set();
  for (const file of walk(staticRoot)) {
    const source = fs.readFileSync(file, "utf8");
    for (const match of source.matchAll(/\/v1\/[A-Za-z0-9_./${}()?-]+/g)) {
      routes.add(normalizeRoute(match[0]));
    }
  }
  return [...routes].sort();
}

test("legacy page API inventory covers every route referenced by preserved assets", () => {
  assert.ok(fs.existsSync(inventoryPath), "missing docs/migration/legacy-page-routes.json");
  const inventory = JSON.parse(fs.readFileSync(inventoryPath, "utf8"));
  const listed = inventory.routes.map((entry) => entry.path).sort();
  assert.deepEqual(listed, discoveredRoutes());
  for (const entry of inventory.routes) {
    assert.match(entry.owner, /^(direct|adapter|port)$/);
    assert.ok(Array.isArray(entry.pages) && entry.pages.length > 0, `missing pages for ${entry.path}`);
  }
});
