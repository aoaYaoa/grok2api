const assert = require("node:assert/strict");
const { execFileSync } = require("node:child_process");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");

const root = path.resolve(__dirname, "..");

function read(relativePath) {
  return fs.readFileSync(path.join(root, relativePath), "utf8");
}

test("compose publishes one Go application and contains no Python or Redis service", () => {
  const compose = read("docker-compose.yml");
  const model = JSON.parse(execFileSync(
    "docker",
    ["compose", "config", "--format", "json"],
    { cwd: root, encoding: "utf8", env: { ...process.env, GROK2API_PORT: "18002" } },
  ));

  assert.match(compose, /^\s{2}grok2api_go:/m);
  assert.doesNotMatch(compose, /^\s{2}grok2api_python:/m);
  assert.doesNotMatch(compose, /^\s{2}grok2api_edge:/m);
  assert.doesNotMatch(compose, /^\s{2}redis:/m);
  assert.doesNotMatch(compose, /container_name:/);
  assert.equal((compose.match(/^\s+ports:/gm) || []).length, 1);

  const published = Object.entries(model.services)
    .filter(([, service]) => Array.isArray(service.ports) && service.ports.length > 0)
    .map(([name]) => name);
  assert.deepEqual(published, ["grok2api_go"]);
  assert.equal(model.services.grok2api_go.ports[0].published, "18002");
  assert.equal(model.services.grok2api_go.build.context, root);
  assert.equal(model.services.grok2api_go.build.dockerfile, "gateway/Dockerfile");

  const goVolumes = model.services.grok2api_go.volumes;
  const configVolume = goVolumes.find((volume) => volume.target === "/run/grok2api/config.yaml");
  assert.ok(configVolume);
  assert.equal(configVolume.read_only, true);
  assert.equal(configVolume.bind.create_host_path, false);
  assert.ok(configVolume.source.endsWith("/gateway-config/config.yaml"));
  const dataVolume = goVolumes.find((volume) => volume.target === "/app/data");
  assert.ok(dataVolume);
  assert.equal(dataVolume.type, "volume");
  assert.match(dataVolume.source, /grok2api_gateway_data$/);

  const health = model.services.grok2api_go.healthcheck.test.join(" ");
  assert.match(health, /\/healthz/);
  assert.doesNotMatch(JSON.stringify(model), /grok2api_python/);
});

test("Go image contains admin and public React builds without legacy page assets", () => {
  const dockerfile = read("gateway/Dockerfile");

  assert.match(dockerfile, /COPY gateway\/frontend\/package\.json gateway\/frontend\/pnpm-lock\.yaml/);
  assert.match(dockerfile, /COPY gateway\/backend\/go\.mod gateway\/backend\/go\.sum/);
  assert.doesNotMatch(dockerfile, /COPY app\/static \/app\/legacy-static/);
  assert.match(dockerfile, /COPY --from=frontend-builder \/src\/frontend\/dist \/app\/frontend\/dist/);
  assert.match(read("gateway/frontend/package.json"), /build:admin/);
  assert.match(read("gateway/frontend/package.json"), /build:public/);
  assert.doesNotMatch(dockerfile, /python/i);
});

test("Go configuration uses SQLite, memory, and direct root API URLs", () => {
  const config = read("gateway/config.example.compat.yaml");
  const gitignore = read(".gitignore");

  assert.match(config, /publicApiBaseURL: "http:\/\/127\.0\.0\.1:18002"/);
  assert.match(config, /staticPath: "\/app\/frontend\/dist"/);
  assert.match(config, /legacy:[\s\S]*staticPath: "\/app\/legacy-static"/);
  assert.match(config, /legacy:[\s\S]*publicEnabled: true/);
  assert.match(config, /database:[\s\S]*driver: sqlite/);
  assert.match(config, /runtimeStore:[\s\S]*driver: memory/);
  assert.match(config, /media:[\s\S]*driver: local/);
  assert.match(config, /socks5:\/\/warp:1080/);
  assert.match(gitignore, /^gateway-config\/$/m);
});
