(function (global) {
  function normalizeApiKey(apiKey) {
    if (!apiKey) return '';
    const trimmed = String(apiKey).trim();
    return trimmed.startsWith('Bearer ') ? trimmed.slice(7).trim() : trimmed;
  }

  function openBatchStream(taskId, apiKey, handlers = {}) {
    if (!taskId) return null;
    const rawKey = normalizeApiKey(apiKey);
    const controller = new AbortController();
    const connection = {
      closed: false,
      close() {
        if (this.closed) return;
        this.closed = true;
        controller.abort();
      }
    };

    (async () => {
      let errorReported = false;
      let terminalEvent = false;
      const reportError = () => {
        if (connection.closed || errorReported) return;
        errorReported = true;
        if (handlers.onError) handlers.onError();
      };
      const dispatch = (block) => {
        const data = block
          .split(/\r?\n/)
          .filter((line) => line.startsWith('data:'))
          .map((line) => line.slice(5).replace(/^ /, ''))
          .join('\n');
        if (!data) return;
        try {
          const message = JSON.parse(data);
          terminalEvent = ['done', 'error', 'cancelled'].includes(message.type);
          if (handlers.onMessage) handlers.onMessage(message);
        } catch {
          // Ignore malformed events, matching the previous EventSource callback.
        }
      };

      try {
        const response = await fetch(`/v1/admin/batch/${encodeURIComponent(taskId)}/stream`, {
          method: 'GET',
          headers: {
            Accept: 'text/event-stream',
            ...(rawKey ? { Authorization: `Bearer ${rawKey}` } : {})
          },
          signal: controller.signal
        });
        if (!response.ok || !response.body) {
          reportError();
          return;
        }

        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        let buffer = '';
        while (!connection.closed) {
          const { done, value } = await reader.read();
          buffer += decoder.decode(value || new Uint8Array(), { stream: !done });
          let match = /\r?\n\r?\n/.exec(buffer);
          while (match) {
            dispatch(buffer.slice(0, match.index));
            buffer = buffer.slice(match.index + match[0].length);
            if (connection.closed) break;
            match = /\r?\n\r?\n/.exec(buffer);
          }
          if (done) {
            if (buffer.trim()) dispatch(buffer);
            if (!terminalEvent) reportError();
            break;
          }
        }
      } catch (error) {
        if (!connection.closed && error?.name !== 'AbortError') reportError();
      }
    })();

    return connection;
  }

  function closeBatchStream(es) {
    if (es) es.close();
  }

  async function cancelBatchTask(taskId, apiKey) {
    if (!taskId) return;
    try {
      const rawKey = normalizeApiKey(apiKey);
      await fetch(`/v1/admin/batch/${encodeURIComponent(taskId)}/cancel`, {
        method: 'POST',
        headers: rawKey ? { Authorization: `Bearer ${rawKey}` } : undefined
      });
    } catch {
      // ignore
    }
  }

  global.BatchSSE = {
    open: openBatchStream,
    close: closeBatchStream,
    cancel: cancelBatchTask
  };
})(window);
