import { afterEach, describe, expect, it, vi } from "vitest";
import type { ApiEnvelope, RealtimeTicket, TaskChangedNotification } from "@flight/contracts";
import { RealtimeClient } from "./index";
import type { RealtimeSocket } from "./index";

class FakeSocket implements RealtimeSocket {
  onopen: ((event: unknown) => void) | null = null;
  onmessage: ((event: { data: unknown }) => void) | null = null;
  onerror: ((event: unknown) => void) | null = null;
  onclose: ((event: unknown) => void) | null = null;
  sent: string[] = [];
  closed = false;

  send(data: string): void { this.sent.push(data); }
  close(): void { this.closed = true; }
}

const ticket: ApiEnvelope<RealtimeTicket> = {
  data: { ticket: "ticket-1", protocol: "flight.realtime.v1", ticket_protocol: "flight.realtime.ticket.ticket-1", expires_at: "2026-09-02T09:00:00Z" },
};

describe("RealtimeClient", () => {
  afterEach(() => vi.useRealTimers());

  it("replies to heartbeat and deduplicates task hints", async () => {
    const sockets: FakeSocket[] = [];
    const changed: TaskChangedNotification[] = [];
    let connected = 0;
    const client = new RealtimeClient({
      webSocketUrl: "ws://edge.test/api/v1/ws",
      getTicket: async () => ticket,
      onConnected: () => { connected += 1; },
      onTaskChanged: (notification) => changed.push(notification),
      webSocketFactory: () => { const socket = new FakeSocket(); sockets.push(socket); return socket; },
      random: () => 1,
    });

    client.start();
    await Promise.resolve();
    sockets[0].onopen?.(undefined);
    sockets[0].onmessage?.({ data: JSON.stringify({ type: "ping" }) });
    const notification = { type: "task_changed", notification_id: "notification-1", task_public_id: "task-1", sync_version: 2, reason: "assigned", issued_at: "2026-09-02T08:00:00Z" } satisfies TaskChangedNotification;
    sockets[0].onmessage?.({ data: JSON.stringify(notification) });
    sockets[0].onmessage?.({ data: JSON.stringify(notification) });

    expect(client.getState()).toBe("open");
    expect(connected).toBe(1);
    expect(JSON.parse(sockets[0].sent[0])).toEqual({ type: "pong" });
    expect(changed).toEqual([notification]);
  });

  it("reconnects with a fresh ticket using bounded backoff", async () => {
    vi.useFakeTimers();
    const sockets: FakeSocket[] = [];
    const client = new RealtimeClient({
      webSocketUrl: "ws://edge.test/api/v1/ws",
      getTicket: async () => ticket,
      onTaskChanged: () => undefined,
      reconnectBaseMs: 100,
      reconnectMaxMs: 200,
      random: () => 1,
      webSocketFactory: () => { const socket = new FakeSocket(); sockets.push(socket); return socket; },
    });

    client.start();
    await vi.runAllTicks();
    sockets[0].onopen?.(undefined);
    sockets[0].onclose?.(undefined);
    expect(client.getState()).toBe("retrying");
    await vi.advanceTimersByTimeAsync(100);
    expect(sockets).toHaveLength(2);
    expect(sockets[0].closed).toBe(false);
    client.stop();
  });
});
