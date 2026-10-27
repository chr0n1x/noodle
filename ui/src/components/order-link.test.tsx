import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { buildOrder, buildSnapshot, buildStage } from "~/test-utils";
import { OrderTitle, orderForSession, safeOrderUrl, sourceLinkLabel } from "./order-link";

describe("safeOrderUrl", () => {
  it("keeps absolute http and https links", () => {
    expect(safeOrderUrl("https://jira.example.com/browse/PROJ-1")).toBe(
      "https://jira.example.com/browse/PROJ-1",
    );
    expect(safeOrderUrl("http://example.com/x")).toBe("http://example.com/x");
  });

  it("drops anything else", () => {
    for (const url of [
      undefined,
      "",
      "PROJ-1",
      "data:text/html,hi",
      "ftp://x/y",
      "https://",
      "/relative",
    ]) {
      expect(safeOrderUrl(url)).toBeUndefined();
    }
  });
});

describe("orderForSession", () => {
  it("finds the order whose stage ran the session", () => {
    const order = buildOrder({ id: "9", stages: [buildStage({ session_id: "s9" })] });
    const snapshot = buildSnapshot({ orders: [buildOrder({ id: "1" }), order] });
    expect(orderForSession(snapshot, "s9")?.id).toBe("9");
    expect(orderForSession(snapshot, "unknown")).toBeUndefined();
  });
});

describe("sourceLinkLabel", () => {
  it("names the host and the last path segment", () => {
    expect(sourceLinkLabel("https://jira.example.com/browse/PROJ-123")).toBe(
      "jira.example.com PROJ-123",
    );
    expect(sourceLinkLabel("https://github.com/o/r/pull/12/")).toBe("github.com 12");
    expect(sourceLinkLabel("https://example.com")).toBe("example.com");
  });
});

describe("OrderTitle", () => {
  it("renders a link that opens in a new tab when the url is usable", () => {
    render(
      <OrderTitle
        title="PROJ-123 fix login bug"
        url="https://jira.example.com/browse/PROJ-123"
      />,
    );
    const link = screen.getByRole("link", { name: "PROJ-123 fix login bug" });
    expect(link).toHaveAttribute("href", "https://jira.example.com/browse/PROJ-123");
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", "noopener noreferrer");
  });

  it("renders plain text without a usable url", () => {
    render(<OrderTitle title="plain title" url="data:text/html,hi" />);
    expect(screen.queryByRole("link")).toBeNull();
    expect(screen.getByText("plain title")).toBeInTheDocument();
  });
});
