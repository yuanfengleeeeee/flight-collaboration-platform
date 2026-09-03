import { describe, expect, it } from "vitest";
import { TaskCommandStore, type TaskCommandReceipt } from "./task-command-store";

function memoryStorage(): Storage {
  const values = new Map<string, string>();
  return {
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => values.set(key, value),
    removeItem: (key) => values.delete(key),
    clear: () => values.clear(),
    key: (index) => [...values.keys()][index] ?? null,
    get length() { return values.size; },
  };
}

const receipt: TaskCommandReceipt = {
  id: "command-1",
  action: "accept",
  taskPublicID: "task-1",
  assignmentPublicID: "assignment-1",
  expectedSyncVersion: 2,
};

describe("TaskCommandStore", () => {
  it("persists a command receipt and reads it only for the matching task", () => {
    const store = new TaskCommandStore(memoryStorage());
    store.write({ ...receipt, status: "pending" });

    expect(store.read("task-1")).toEqual({ ...receipt, status: "pending" });
    expect(store.read("task-2")).toBeUndefined();
  });

  it("does not clear a newer command when an old command completes", () => {
    const store = new TaskCommandStore(memoryStorage());
    store.write(receipt);
    store.write({ ...receipt, id: "command-2", action: "complete", taskPublicID: "task-2" });

    store.clear("command-1");

    expect(store.read()).toMatchObject({ id: "command-2", taskPublicID: "task-2" });
  });
});
