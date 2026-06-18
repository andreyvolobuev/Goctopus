// Goctopus browser/Node client.
//
// Handles the three things every Goctopus consumer needs: ACKing each message
// by id, de-duplicating re-deliveries (delivery is at-least-once), and
// reconnecting with backoff.
//
// Browser:
//   const c = new GoctopusClient("wss://goctopus.example.com", {
//     onMessage: (payload, id) => console.log(payload),
//   });
//   c.connect();
//
// Node (npm i ws):
//   const WebSocket = require("ws");
//   const c = new GoctopusClient("ws://localhost:7890", { WebSocket, onMessage });

class GoctopusClient {
  constructor(url, opts = {}) {
    this.url = url;
    this.onMessage = opts.onMessage || (() => {});
    this.onOpen = opts.onOpen || (() => {});
    this.onClose = opts.onClose || (() => {});
    // Allow injecting a WebSocket implementation (Node), default to global.
    this.WebSocket = opts.WebSocket || (typeof WebSocket !== "undefined" ? WebSocket : null);
    this.minBackoff = opts.minBackoff || 500;
    this.maxBackoff = opts.maxBackoff || 10000;
    this.maxRetries = opts.maxRetries == null ? Infinity : opts.maxRetries;
    this.dedupeLimit = opts.dedupeLimit || 1000;

    this._seen = new Set();
    this._seenOrder = [];
    this._backoff = this.minBackoff;
    this._retries = 0;
    this._stopped = false;
    this._socket = null;
  }

  connect() {
    if (!this.WebSocket) throw new Error("no WebSocket implementation available");
    this._stopped = false;
    this._open();
    return this;
  }

  close() {
    this._stopped = true;
    if (this._socket) this._socket.close();
  }

  _open() {
    const socket = new this.WebSocket(this.url);
    this._socket = socket;

    socket.onopen = () => {
      this._backoff = this.minBackoff;
      this._retries = 0;
      this.onOpen();
    };

    socket.onmessage = (event) => {
      let d;
      try {
        d = JSON.parse(event.data);
      } catch (e) {
        return;
      }
      // ACK first so the server stops re-delivering.
      try {
        socket.send(JSON.stringify({ id: d.id }));
      } catch (e) {
        /* socket closing; it will be re-delivered */
      }
      if (this._markSeen(d.id)) {
        this.onMessage(d.payload, d.id);
      }
    };

    socket.onclose = () => {
      this.onClose();
      if (this._stopped) return;
      if (this._retries >= this.maxRetries) return;
      this._retries++;
      // Exponential backoff with full jitter to avoid reconnect stampedes.
      const delay = Math.random() * this._backoff;
      setTimeout(() => this._open(), delay);
      this._backoff = Math.min(this._backoff * 2, this.maxBackoff);
    };

    socket.onerror = () => socket.close();
  }

  // Returns true if id is new (and records it), false if already seen.
  _markSeen(id) {
    if (id == null) return true;
    if (this._seen.has(id)) return false;
    this._seen.add(id);
    this._seenOrder.push(id);
    if (this._seenOrder.length > this.dedupeLimit) {
      this._seen.delete(this._seenOrder.shift());
    }
    return true;
  }
}

if (typeof module !== "undefined" && module.exports) {
  module.exports = { GoctopusClient };
}
