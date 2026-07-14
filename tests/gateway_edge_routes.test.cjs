const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");

const root = path.resolve(__dirname, "..");

function read(relativePath) {
  return fs.readFileSync(path.join(root, relativePath), "utf8");
}

test("hybrid compose isolates Python and Go behind one edge without Redis", () => {
  const compose = read("docker-compose.yml");

  assert.match(compose, /^\s{2}grok2api_edge:/m);
  assert.match(compose, /^\s{2}grok2api_python:/m);
  assert.match(compose, /^\s{2}grok2api_go:/m);
  assert.match(compose, /\$\{GROK2API_PORT:-18000\}:8000/);
  assert.doesNotMatch(compose, /container_name:/);
  assert.doesNotMatch(compose, /^\s{2}redis:/m);
  assert.match(compose, /\.\/data\/gateway\/config\.yaml:\/run\/grok2api\/config\.yaml:ro/);
  assert.match(compose, /\.\/data\/gateway:\/app\/data/);
});

test("edge keeps legacy routes on Python and mounts Go below gateway", () => {
  const nginx = read("nginx/conf/hybrid.conf");

  assert.match(nginx, /location = \/gateway\s*\{[\s\S]*return 308 \/gateway\//);
  assert.match(nginx, /location = \/gateway\/healthz\s*\{[\s\S]*proxy_pass http:\/\/grok2api_go\/healthz/);
  assert.match(nginx, /location = \/gateway\/readyz\s*\{[\s\S]*proxy_pass http:\/\/grok2api_go\/readyz/);
  assert.match(nginx, /location \^~ \/gateway\/v1\//);
  assert.ok(nginx.includes("rewrite ^/gateway(/v1/.*)$ $1 break;"));
  assert.match(nginx, /location \^~ \/api\/admin\/v1\/\s*\{[\s\S]*proxy_pass http:\/\/grok2api_go/);
  assert.match(nginx, /location \^~ \/gateway\//);
  assert.ok(nginx.includes("rewrite ^/gateway/(.*)$ /$1 break;"));
  assert.match(nginx, /location \^~ \/internal\/\s*\{\s*return 404;\s*\}/);
  assert.match(nginx, /location \/\s*\{[\s\S]*proxy_pass http:\/\/grok2api_python/);
  assert.match(nginx, /proxy_buffering off;/);
  assert.match(nginx, /proxy_read_timeout 7200s;/);
});

test("Go compatibility config is single-instance memory and local storage", () => {
  const config = read("gateway/config.example.compat.yaml");
  const gitignore = read(".gitignore");

  assert.match(config, /publicApiBaseURL: "http:\/\/127\.0\.0\.1:18000\/gateway"/);
  assert.match(config, /staticPath: "\/app\/frontend\/dist"/);
  assert.match(config, /database:[\s\S]*driver: sqlite/);
  assert.match(config, /runtimeStore:[\s\S]*driver: memory/);
  assert.match(config, /media:[\s\S]*driver: local/);
  assert.match(config, /socks5:\/\/warp:1080/);
  assert.match(gitignore, /^data\/gateway\/$/m);
});
