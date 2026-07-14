const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");

const root = path.resolve(__dirname, "..");

test("preserved chat page uses Go legacy-authenticated model and completion routes", () => {
  const source = fs.readFileSync(path.join(root, "app/static/public/js/chat.js"), "utf8");
  assert.match(source, /fetch\('\/v1\/public\/models'/);
  assert.match(source, /fetch\('\/v1\/public\/chat\/completions'/);
  assert.doesNotMatch(source, /fetch\('\/v1\/models'/);
});
