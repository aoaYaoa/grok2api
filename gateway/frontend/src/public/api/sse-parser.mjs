export function createSSEParser() {
  let buffer = "";
  return {
    push(chunk) {
      buffer += String(chunk || "").replaceAll("\r\n", "\n");
      const output = [];
      let boundary = buffer.indexOf("\n\n");
      while (boundary >= 0) {
        const frame = buffer.slice(0, boundary);
        buffer = buffer.slice(boundary + 2);
        let event = "message";
        const data = [];
        for (const line of frame.split("\n")) {
          if (line.startsWith("event:")) event = line.slice(6).trim() || "message";
          if (line.startsWith("data:")) data.push(line.slice(5).trimStart());
        }
        if (data.length) {
          const raw = data.join("\n");
          try {
            output.push({ event, data: JSON.parse(raw) });
          } catch {
            output.push({ event, data: raw });
          }
        }
        boundary = buffer.indexOf("\n\n");
      }
      return output;
    },
  };
}
