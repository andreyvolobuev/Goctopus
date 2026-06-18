"""Goctopus asyncio client.

Handles ACKing by id, de-duplicating re-deliveries (delivery is at-least-once)
and reconnecting with backoff.

Requires the `websockets` package:  pip install websockets

Example:

    import asyncio
    from goctopus_client import GoctopusClient

    async def main():
        async def on_message(payload, msg_id):
            print("got", payload)

        client = GoctopusClient("ws://localhost:7890", on_message=on_message)
        await client.run()

    asyncio.run(main())
"""

import asyncio
import json
from collections import deque

import websockets


class GoctopusClient:
    def __init__(
        self,
        url,
        on_message,
        *,
        min_backoff=0.5,
        max_backoff=10.0,
        dedupe_limit=1000,
        extra_headers=None,
    ):
        self.url = url
        self.on_message = on_message
        self.min_backoff = min_backoff
        self.max_backoff = max_backoff
        self.extra_headers = extra_headers
        self._seen = set()
        self._seen_order = deque()
        self._dedupe_limit = dedupe_limit
        self._stopped = False

    async def run(self):
        """Connect and keep reconnecting until stop() is called."""
        backoff = self.min_backoff
        while not self._stopped:
            try:
                async with websockets.connect(
                    self.url, extra_headers=self.extra_headers
                ) as ws:
                    backoff = self.min_backoff
                    await self._read_loop(ws)
            except Exception:
                if self._stopped:
                    break
                await asyncio.sleep(backoff)
                backoff = min(backoff * 2, self.max_backoff)

    def stop(self):
        self._stopped = True

    async def _read_loop(self, ws):
        async for raw in ws:
            try:
                d = json.loads(raw)
            except (ValueError, TypeError):
                continue
            # ACK first so the server stops re-delivering.
            try:
                await ws.send(json.dumps({"id": d.get("id")}))
            except Exception:
                pass
            if self._mark_seen(d.get("id")):
                result = self.on_message(d.get("payload"), d.get("id"))
                if asyncio.iscoroutine(result):
                    await result

    def _mark_seen(self, msg_id):
        if msg_id is None:
            return True
        if msg_id in self._seen:
            return False
        self._seen.add(msg_id)
        self._seen_order.append(msg_id)
        if len(self._seen_order) > self._dedupe_limit:
            self._seen.discard(self._seen_order.popleft())
        return True
