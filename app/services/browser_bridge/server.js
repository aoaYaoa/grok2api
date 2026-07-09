"use strict";

const http = require("http");
const fs = require("fs");
const path = require("path");
const { chromium } = require("playwright");

const HOST = process.env.GROK_CLOAK_HOST || "127.0.0.1";
const PORT = parseInt(process.env.GROK_CLOAK_PORT || "9081", 10);
const NAV_TIMEOUT = parseInt(process.env.GROK_CLOAK_NAV_TIMEOUT_MS || "45000", 10);
const READY_TIMEOUT = parseInt(process.env.GROK_CLOAK_READY_TIMEOUT_MS || "30000", 10);
const REQUEST_TIMEOUT = parseInt(process.env.GROK_CLOAK_REQUEST_TIMEOUT_MS || "120000", 10);
const PAGE_IDLE_MS = parseInt(process.env.GROK_CLOAK_IDLE_PAGE_MS || "300000", 10);
const MAX_PAGES = parseInt(process.env.GROK_CLOAK_MAX_PAGES || "4", 10);
const HEADLESS = String(process.env.GROK_CLOAK_HEADLESS || "true").toLowerCase() !== "false";
const EXECUTABLE_PATH = (process.env.GROK_CLOAK_EXECUTABLE_PATH || "").trim();
const PROFILE_DIR = (process.env.GROK_CLOAK_PROFILE_DIR || "").trim();

let browser = null;
const pages = new Map();
let lastError = null;

class BridgeError extends Error {
  constructor(code, message, status = 502) {
    super(message);
    this.name = "BridgeError";
    this.code = code;
    this.status = status;
  }
}

function log(message) {
  console.log(`[cloakbridge] ${new Date().toISOString()} ${message}`);
}

function ensureProfileDir() {
  if (!PROFILE_DIR) return "";
  fs.mkdirSync(PROFILE_DIR, { recursive: true });
  return PROFILE_DIR;
}

async function ensureBrowser() {
  if (browser) {
    if (typeof browser.isConnected === "function") {
      if (browser.isConnected()) return browser;
    } else {
      return browser;
    }
  }

  const launchOptions = {
    headless: HEADLESS,
    args: [
      "--disable-blink-features=AutomationControlled",
      "--disable-dev-shm-usage",
      "--no-sandbox",
    ],
  };
  if (EXECUTABLE_PATH) {
    launchOptions.executablePath = EXECUTABLE_PATH;
  }

  const userDataDir = ensureProfileDir();
  if (userDataDir) {
    browser = await chromium.launchPersistentContext(userDataDir, launchOptions);
    browser.on("close", () => {
      browser = null;
      pages.clear();
    });
    return browser;
  }

  const launched = await chromium.launch(launchOptions);
  launched.on("disconnected", () => {
    browser = null;
    pages.clear();
  });
  browser = launched;
  return browser;
}

async function createContextWithCookies(playwrightBrowser, sso) {
  const isPersistentContext =
    typeof playwrightBrowser.newPage === "function" &&
    typeof playwrightBrowser.cookies === "function" &&
    typeof playwrightBrowser.newContext !== "function";

  if (isPersistentContext) {
    await playwrightBrowser.addCookies([
      { name: "sso", value: sso, domain: ".grok.com", path: "/" },
      { name: "sso-rw", value: sso, domain: ".grok.com", path: "/" },
    ]);
    return { context: playwrightBrowser, page: await playwrightBrowser.newPage() };
  }

  const context = await playwrightBrowser.newContext({
    viewport: { width: 1600, height: 1000 },
  });
  await context.addCookies([
    { name: "sso", value: sso, domain: ".grok.com", path: "/" },
    { name: "sso-rw", value: sso, domain: ".grok.com", path: "/" },
  ]);
  return { context, page: await context.newPage() };
}

async function prepareSlot(slot) {
  const { page } = slot;
  await page.goto("https://grok.com/", {
    waitUntil: "domcontentloaded",
    timeout: NAV_TIMEOUT,
  });

  await page.waitForLoadState("networkidle", { timeout: READY_TIMEOUT }).catch(() => {});
  await page.locator("textarea, [contenteditable='true']").first().waitFor({
    state: "visible",
    timeout: READY_TIMEOUT,
  });

  const cookies = await slot.context.cookies("https://grok.com");
  const hasSession = cookies.some((item) => item.name === "x-userid" || item.name === "sso");
  if (!hasSession) {
    throw new BridgeError("sso_unavailable", "SSO cookie did not establish a Grok session", 401);
  }
  slot.ready = true;
  slot.lastUsed = Date.now();
}

async function destroySlot(key) {
  const slot = pages.get(key);
  if (!slot) return;
  pages.delete(key);
  try {
    if (slot.context && slot.context !== browser) {
      await slot.context.close();
    } else if (slot.page) {
      await slot.page.close();
    }
  } catch (_) {}
}

async function getSlot(sso) {
  let slot = pages.get(sso);
  if (slot && slot.ready && !slot.busy) {
    slot.lastUsed = Date.now();
    return slot;
  }
  if (slot) {
    await destroySlot(sso);
  }

  if (pages.size >= MAX_PAGES) {
    const oldest = [...pages.entries()]
      .filter(([, value]) => !value.busy)
      .sort((a, b) => a[1].lastUsed - b[1].lastUsed)[0];
    if (oldest) {
      await destroySlot(oldest[0]);
    }
  }

  const b = await ensureBrowser();
  const { context, page } = await createContextWithCookies(b, sso);
  slot = {
    sso,
    context,
    page,
    ready: false,
    busy: false,
    lastUsed: Date.now(),
  };
  pages.set(sso, slot);
  await prepareSlot(slot);
  return slot;
}

async function requestViaPage(slot, payload, conversationId) {
  const targetUrl = conversationId
    ? `https://grok.com/rest/app-chat/conversations/${conversationId}/responses`
    : "https://grok.com/rest/app-chat/conversations/new";
  const referer = conversationId ? `https://grok.com/c/${conversationId}` : "https://grok.com/";

  const response = await slot.page.evaluate(
    async ({ url, reqPayload, reqReferer }) => {
      const resp = await fetch(url, {
        method: "POST",
        credentials: "include",
        headers: {
          "Content-Type": "application/json",
          "Accept": "*/*",
          "Origin": "https://grok.com",
          "Referer": reqReferer,
        },
        body: JSON.stringify(reqPayload),
      });
      const text = await resp.text();
      return {
        status: resp.status,
        body: text,
      };
    },
    { url: targetUrl, reqPayload: payload, reqReferer: referer }
  );

  if (!response || typeof response.status !== "number") {
    throw new BridgeError("upstream_invalid", "Browser returned an invalid upstream response", 502);
  }
  log(
    `upstream status=${response.status} conversation=${conversationId || "-"} body=${JSON.stringify(
      String(response.body || "").slice(0, 400)
    )}`
  );
  return response;
}

async function sendMessage(sso, payload, conversationId) {
  const slot = await getSlot(sso);
  if (slot.busy) {
    throw new BridgeError("page_busy", "Bridge page busy", 429);
  }

  slot.busy = true;
  slot.lastUsed = Date.now();
  try {
    const result = await Promise.race([
      requestViaPage(slot, payload, conversationId),
      new Promise((_, reject) =>
        setTimeout(() => reject(new BridgeError("request_timeout", "Bridge request timeout", 504)), REQUEST_TIMEOUT)
      ),
    ]);
    return result;
  } finally {
    slot.busy = false;
    slot.lastUsed = Date.now();
  }
}

setInterval(async () => {
  const now = Date.now();
  for (const [key, slot] of pages.entries()) {
    if (!slot.busy && now - slot.lastUsed > PAGE_IDLE_MS) {
      await destroySlot(key);
    }
  }
}, 60000);

function respond(res, status, payload, contentType = "application/json") {
  const body = typeof payload === "string" ? payload : JSON.stringify(payload);
  res.writeHead(status, {
    "Content-Type": contentType,
    "Content-Length": Buffer.byteLength(body),
  });
  res.end(body);
}

function readJson(req) {
  return new Promise((resolve, reject) => {
    const chunks = [];
    req.on("data", (chunk) => chunks.push(chunk));
    req.on("end", () => {
      try {
        resolve(JSON.parse(Buffer.concat(chunks).toString("utf-8")));
      } catch (error) {
        reject(error);
      }
    });
    req.on("error", reject);
  });
}

const server = http.createServer(async (req, res) => {
  if (req.method === "GET" && req.url === "/health") {
    return respond(res, 200, {
      status: lastError ? "degraded" : "ok",
      pages: pages.size,
      last_error: lastError,
    });
  }

  if (req.method === "POST" && req.url === "/api/chat") {
    try {
      const body = await readJson(req);
      const sso = typeof body.sso === "string" ? body.sso.trim() : "";
      const payload = body.payload;
      const conversationId =
        typeof body.conversation_id === "string" ? body.conversation_id.trim() : "";
      if (!sso) {
        throw new BridgeError("invalid_request", "Missing sso", 400);
      }
      if (!payload || typeof payload !== "object") {
        throw new BridgeError("invalid_request", "Missing payload", 400);
      }

      const result = await sendMessage(sso, payload, conversationId);
      if (result.status >= 400) {
        lastError = { code: "upstream_error", status: result.status };
        log(`upstream_error status=${result.status} body=${JSON.stringify(String(result.body || "").slice(0, 400))}`);
        return respond(res, result.status, {
          code: "upstream_error",
          error: `Upstream returned ${result.status}`,
          body: result.body,
        });
      }

      lastError = null;
      return respond(res, 200, result.body, "text/plain; charset=utf-8");
    } catch (error) {
      const bridgeError =
        error instanceof BridgeError
          ? error
          : new BridgeError("bridge_failed", error.message || "Bridge failed", 502);
      lastError = { code: bridgeError.code, message: bridgeError.message };
      log(`request failed: ${bridgeError.code} ${bridgeError.message}`);
      return respond(res, bridgeError.status, {
        code: bridgeError.code,
        error: bridgeError.message,
      });
    }
  }

  return respond(res, 404, { code: "not_found", error: "Not found" });
});

server.listen(PORT, HOST, () => {
  log(`listening on ${HOST}:${PORT}`);
});
