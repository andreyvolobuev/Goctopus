export interface GoctopusOptions {
  onMessage?: (payload: unknown, id: string) => void;
  onOpen?: () => void;
  onClose?: () => void;
  /** WebSocket implementation to use (e.g. `ws` in Node). Defaults to global WebSocket. */
  WebSocket?: unknown;
  minBackoff?: number;
  maxBackoff?: number;
  /** Maximum reconnect attempts before giving up. Defaults to Infinity. */
  maxRetries?: number;
  /** Number of recent message ids remembered for de-duplication. */
  dedupeLimit?: number;
}

export class GoctopusClient {
  constructor(url: string, opts?: GoctopusOptions);
  connect(): this;
  close(): void;
}
