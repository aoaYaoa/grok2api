export interface SSEFrame<T = unknown> { event: string; data: T }
export function createSSEParser(): { push(chunk: string): SSEFrame[] };
