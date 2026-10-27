import type { Snapshot, Session, Order } from "~/client";

export interface TreeNodeData {
  name: string;
  type: "scheduler" | "order" | "stage";
  status: string;
  cost?: number;
  model?: string;
  currentAction?: string;
  sessionId?: string;
  children?: TreeNodeData[];
}

export function snapshotToHierarchy(snapshot: Snapshot): TreeNodeData {
  return {
    name: "Scheduler",
    type: "scheduler",
    status: snapshot.loop_state,
    children: snapshot.orders.map((order) => orderToNode(order, snapshot.sessions)),
  };
}

// The loop runs one stage of an order at a time, in index order, so each stage
// hangs under the one before it. Orders run side by side under the scheduler.
function orderToNode(order: Order, sessions: Session[]): TreeNodeData {
  const stages: TreeNodeData[] = order.stages.map((stage) => {
    const session = stage.session_id ? sessions.find((s) => s.id === stage.session_id) : undefined;
    return {
      name: stage.task_key || "stage",
      type: "stage" as const,
      status: stage.status,
      cost: session?.total_cost_usd,
      model: session?.model,
      currentAction: session?.current_action,
      sessionId: stage.session_id,
    };
  });
  for (let i = stages.length - 2; i >= 0; i--) {
    stages[i].children = [stages[i + 1]];
  }
  return {
    name: order.title || order.id,
    type: "order",
    status: order.status,
    children: stages.length > 0 ? [stages[0]] : [],
  };
}
