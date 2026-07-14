const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

const source = fs.readFileSync(path.join(__dirname, 'batch-sse.js'), 'utf8');

function loadBatchSSE(fetch) {
  const window = {};
  vm.runInNewContext(source, {
    AbortController,
    EventSource: undefined,
    TextDecoder,
    fetch,
    window
  });
  return window.BatchSSE;
}

function streamResponse(chunks, options = {}) {
  const encoder = new TextEncoder();
  return {
    ok: options.ok ?? true,
    status: options.status ?? 200,
    body: new ReadableStream({
      start(controller) {
        for (const chunk of chunks) controller.enqueue(encoder.encode(chunk));
        controller.close();
      }
    })
  };
}

test('open authenticates with a bearer header and parses chunked SSE events', async () => {
  const calls = [];
  const messages = [];
  let resolveDone;
  const done = new Promise((resolve) => { resolveDone = resolve; });
  const BatchSSE = loadBatchSSE(async (url, options) => {
    calls.push({ url, options });
    return streamResponse([
      ': ping\n\ndata: {"type":"snap',
      'shot","processed":0}\n\ndata: not-json\n\n',
      'data: {"type":"done",\n',
      'data: "processed":1}\n\n'
    ]);
  });

  const connection = BatchSSE.open('task/with spaces', ' Bearer admin-secret ', {
    onMessage(message) {
      messages.push(message);
      if (message.type === 'done') resolveDone();
    }
  });

  assert.equal(typeof connection.close, 'function');
  await done;
  assert.equal(calls.length, 1);
  assert.equal(calls[0].url, '/v1/admin/batch/task%2Fwith%20spaces/stream');
  assert.equal(calls[0].options.headers.Authorization, 'Bearer admin-secret');
  assert.equal(calls[0].options.signal.aborted, false);
  assert.equal(JSON.stringify(messages), JSON.stringify([
    { type: 'snapshot', processed: 0 },
    { type: 'done', processed: 1 }
  ]));
});

test('open reports HTTP failures through the existing error callback', async () => {
  let resolveError;
  const error = new Promise((resolve) => { resolveError = resolve; });
  const BatchSSE = loadBatchSSE(async () => streamResponse([], { ok: false, status: 401 }));

  BatchSSE.open('task-1', 'admin-secret', { onError: resolveError });

  await error;
});

test('close aborts the authenticated stream without reporting an error', async () => {
  let signal;
  let errorCalls = 0;
  const BatchSSE = loadBatchSSE((_url, options) => {
    signal = options.signal;
    return new Promise((_resolve, reject) => {
      signal.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError')));
    });
  });

  const connection = BatchSSE.open('task-1', 'admin-secret', {
    onError() { errorCalls += 1; }
  });
  BatchSSE.close(connection);
  await new Promise((resolve) => setTimeout(resolve, 0));

  assert.equal(signal.aborted, true);
  assert.equal(errorCalls, 0);
});

test('cancel preserves bearer authentication and omits credentials from the URL', async () => {
  const calls = [];
  const BatchSSE = loadBatchSSE(async (url, options) => {
    calls.push({ url, options });
    return { ok: true };
  });

  await BatchSSE.cancel('task/1', 'Bearer admin-secret');

  assert.equal(calls[0].url, '/v1/admin/batch/task%2F1/cancel');
  assert.equal(calls[0].options.method, 'POST');
  assert.equal(calls[0].options.headers.Authorization, 'Bearer admin-secret');
});
