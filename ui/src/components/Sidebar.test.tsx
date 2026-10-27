import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Sidebar } from "./Sidebar";
import { buildSnapshot, buildOrder, buildStage, buildSession } from "../test-utils";
import type { Snapshot, ChannelId } from "~/client";

const mockSetActiveChannel = vi.fn();
const mockNavigate = vi.fn();
let mockActiveChannel: ChannelId = { type: "scheduler" };
let mockSnapshot: Snapshot = buildSnapshot();
let mockPathname = "/";

vi.mock("~/client", async () => {
  const actual = await vi.importActual("~/client");
  return {
    ...(actual as object),
    useSuspenseSnapshot: () => ({ data: mockSnapshot }),
    useWSStatus: () => "connected" as const,
    useSendControl: () => ({ mutate: vi.fn(), isPending: false }),
    useActiveChannel: () => ({
      activeChannel: mockActiveChannel,
      setActiveChannel: mockSetActiveChannel,
    }),
  };
});

vi.mock("@tanstack/react-router", () => ({
  useLocation: () => ({ pathname: mockPathname }),
  useNavigate: () => mockNavigate,
  Link: ({
    to,
    children,
    className,
  }: {
    to: string;
    children: React.ReactNode;
    className?: string;
  }) => (
    <a href={to} className={className}>
      {children}
    </a>
  ),
}));

beforeEach(() => {
  mockActiveChannel = { type: "scheduler" };
  mockSnapshot = buildSnapshot();
  mockPathname = "/";
  mockSetActiveChannel.mockClear();
  mockNavigate.mockClear();
});

describe("Sidebar", () => {
  it("renders nav links", () => {
    render(<Sidebar />);
    expect(screen.getByText("Dashboard")).toBeInTheDocument();
    expect(screen.getByText("Live Feed")).toBeInTheDocument();
    expect(screen.getByText("Topology")).toBeInTheDocument();
  });

  it("renders scheduler channel item", () => {
    render(<Sidebar />);
    expect(screen.getByText("Scheduler")).toBeInTheDocument();
  });

  it("renders orders from snapshot", () => {
    mockSnapshot = buildSnapshot({
      orders: [
        buildOrder({
          id: "order-1",
          title: "Fix auth bug",
          status: "active",
          stages: [
            buildStage({ status: "completed", task_key: "test", session_id: "s1" }),
            buildStage({ status: "active", task_key: "execute", session_id: "s2" }),
          ],
        }),
      ],
    });
    render(<Sidebar />);
    expect(screen.getByText("Fix auth bug")).toBeInTheDocument();
  });

  it("links an order to its source url", () => {
    mockSnapshot = buildSnapshot({
      orders: [
        buildOrder({
          id: "9",
          title: "PROJ-123 fix login bug",
          url: "https://jira.example.com/browse/PROJ-123",
          stages: [buildStage({})],
        }),
        buildOrder({
          id: "10",
          title: "No source",
          url: "data:text/html,hi",
          stages: [buildStage({})],
        }),
      ],
    });
    render(<Sidebar />);
    const links = screen.getAllByRole("link", { name: /jira\.example\.com PROJ-123/ });
    expect(links).toHaveLength(1);
    expect(links[0]).toHaveAttribute("href", "https://jira.example.com/browse/PROJ-123");
    expect(screen.queryByRole("link", { name: /data:/ })).not.toBeInTheDocument();
  });

  it("links orders that have a draft PR", () => {
    mockSnapshot = buildSnapshot({
      orders: [buildOrder({ id: "1", title: "Fix auth bug", stages: [buildStage({})] })],
      draft_prs: { "1": { branch: "noodle/1", url: "https://github.com/a/b/pull/2" } },
    });
    render(<Sidebar />);
    expect(screen.getByRole("link", { name: /PR #2/ })).toHaveAttribute(
      "href",
      "https://github.com/a/b/pull/2",
    );
  });

  it("renders repeated stage signatures as distinct indexed labels", async () => {
    mockSnapshot = buildSnapshot({
      orders: [
        buildOrder({
          id: "order-dup",
          title: "Duplicate signature order",
          status: "active",
          stages: [
            buildStage({ status: "completed", task_key: "execute" }),
            buildStage({ status: "completed", task_key: "execute" }),
            buildStage({ status: "pending", task_key: "execute" }),
            buildStage({ status: "pending", task_key: "execute" }),
          ],
        }),
      ],
    });
    render(<Sidebar />);
    const user = userEvent.setup();
    await user.click(screen.getByText("Duplicate signature order"));
    expect(screen.getByText("execute 1")).toBeInTheDocument();
    expect(screen.getByText("execute 2")).toBeInTheDocument();
    expect(screen.getByText("execute 3")).toBeInTheDocument();
    expect(screen.getByText("execute 4")).toBeInTheDocument();
  });

  it("shows schedule bootstrap order in sidebar during bootstrap session", () => {
    mockSnapshot = buildSnapshot({
      sessions: [
        buildSession({
          id: "bootstrap-schedule-123",
          task_key: "schedule",
          status: "running",
        }),
      ],
      orders: [
        buildOrder({
          id: "schedule",
          title: "scheduling tasks based on your backlog",
          status: "active",
          stages: [buildStage({ status: "pending", task_key: "schedule" })],
        }),
      ],
    });

    render(<Sidebar />);
    expect(screen.getByText("Bootstrapping schedule skill")).toBeInTheDocument();
  });

  it("shows schedule bootstrap order in sidebar while bootstrap is pending", () => {
    mockSnapshot = buildSnapshot({
      loop_state: "running",
      sessions: [],
      orders: [
        buildOrder({
          id: "schedule",
          title: "scheduling tasks based on your backlog",
          status: "active",
          stages: [buildStage({ status: "pending", task_key: "schedule" })],
        }),
      ],
    });

    render(<Sidebar />);
    expect(screen.getByText("Bootstrapping schedule skill")).toBeInTheDocument();
  });

  it("navigates to / when Scheduler clicked", async () => {
    mockActiveChannel = { type: "agent", sessionId: "s1" };
    render(<Sidebar />);
    const user = userEvent.setup();
    await user.click(screen.getByText("Scheduler"));
    expect(mockNavigate).toHaveBeenCalledWith({ to: "/" });
  });

  it("navigates to actor route when stage with session_id clicked", async () => {
    mockSnapshot = buildSnapshot({
      orders: [
        buildOrder({
          id: "order-1",
          title: "Test order",
          status: "active",
          stages: [buildStage({ status: "active", task_key: "execute", session_id: "s1" })],
        }),
      ],
    });
    render(<Sidebar />);
    const user = userEvent.setup();
    // Click to expand the order first
    await user.click(screen.getByText("Test order"));
    await user.click(screen.getByText("execute 1"));
    expect(mockNavigate).toHaveBeenCalledWith({ to: "/actor/$id", params: { id: "s1" } });
  });

  it("navigates to bootstrap actor when clicking schedule stage during bootstrap", async () => {
    mockSnapshot = buildSnapshot({
      sessions: [
        buildSession({
          id: "bootstrap-schedule-123",
          task_key: "schedule",
          status: "running",
        }),
      ],
      orders: [
        buildOrder({
          id: "schedule",
          title: "scheduling tasks based on your backlog",
          status: "active",
          stages: [buildStage({ status: "pending", task_key: "schedule" })],
        }),
      ],
    });

    render(<Sidebar />);
    const user = userEvent.setup();
    await user.click(screen.getByText("Bootstrapping schedule skill"));
    await user.click(screen.getByText("schedule 1"));
    expect(mockNavigate).toHaveBeenCalledWith({
      to: "/actor/$id",
      params: { id: "bootstrap-schedule-123" },
    });
  });

  it("displays total cost in footer", () => {
    mockSnapshot = buildSnapshot({ total_cost_usd: 3.45 });
    render(<Sidebar />);
    expect(screen.getByText("$3.45")).toBeInTheDocument();
  });

  it("highlights active nav link", () => {
    render(<Sidebar />);
    const liveFeed = screen.getByText("Live Feed");
    const link = liveFeed.closest("a");
    expect(link?.className).toContain("active");
  });

  it("navigates to actor route from non-feed page", async () => {
    mockPathname = "/dashboard";
    mockSnapshot = buildSnapshot({
      orders: [
        buildOrder({
          id: "order-1",
          title: "Test order",
          status: "active",
          stages: [buildStage({ status: "active", task_key: "execute", session_id: "s1" })],
        }),
      ],
    });
    render(<Sidebar />);
    const user = userEvent.setup();
    // Click to expand the order first
    await user.click(screen.getByText("Test order"));
    await user.click(screen.getByText("execute 1"));
    expect(mockNavigate).toHaveBeenCalledWith({ to: "/actor/$id", params: { id: "s1" } });
  });

  it("navigates to / when Scheduler clicked from feed page", async () => {
    mockPathname = "/";
    render(<Sidebar />);
    const user = userEvent.setup();
    await user.click(screen.getByText("Scheduler"));
    expect(mockNavigate).toHaveBeenCalledWith({ to: "/" });
  });

  it("renders logo mark", () => {
    const { container } = render(<Sidebar />);
    expect(container.querySelector(".logo-mark")).toBeTruthy();
  });

  it("renders agent avatar for scheduler", () => {
    const { container } = render(<Sidebar />);
    const avatar = container.querySelector(".agent-avatar");
    expect(avatar).toBeTruthy();
    expect(avatar?.textContent).toBe("S");
  });

  it("shows an awaiting publish badge on awaiting-publish orders", () => {
    mockSnapshot = buildSnapshot({
      orders: [
        buildOrder({
          id: "o-1",
          title: "Waiting",
          status: "awaiting-publish",
          stages: [buildStage({ status: "completed" })],
        }),
        buildOrder({ id: "o-2", title: "Running", status: "active" }),
      ],
    });
    render(<Sidebar />);
    expect(screen.getAllByText("awaiting publish")).toHaveLength(1);
  });

  it("shows Approve & Publish only for gated orders", () => {
    mockSnapshot = buildSnapshot({
      orders: [buildOrder({ id: "o-1", title: "Ungated", status: "active" })],
    });
    const { unmount } = render(<Sidebar />);
    expect(screen.queryByRole("button", { name: "Approve & Publish" })).not.toBeInTheDocument();
    unmount();

    mockSnapshot = buildSnapshot({
      orders: [
        buildOrder({ id: "o-1", title: "Gated", status: "awaiting-publish", publish_gated: true }),
      ],
    });
    render(<Sidebar />);
    expect(screen.getByRole("button", { name: "Approve & Publish" })).toBeEnabled();
  });

  it("draws cancelled (skipped) stages dimmed, not as pending", async () => {
    mockSnapshot = buildSnapshot({
      orders: [
        buildOrder({
          id: "order-skip",
          title: "Skipped stages order",
          status: "active",
          stages: [
            buildStage({ status: "completed", task_key: "execute" }),
            buildStage({ status: "cancelled", task_key: "quality" }),
          ],
        }),
      ],
    });
    render(<Sidebar />);
    await userEvent.setup().click(screen.getByText("Skipped stages order"));
    const stage = screen.getByText("quality 2").closest(".tree-stage");
    expect(stage).toHaveClass("stage-skipped");
    expect(stage).not.toHaveClass("stage-pending");
  });
});
