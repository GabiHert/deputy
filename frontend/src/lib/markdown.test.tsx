import { render } from "@testing-library/react";
import { Markdown, parseInline, parseMarkdown } from "./markdown";

describe("markdown", () => {
  it("parses blocks and inline marks", () => {
    const b = parseMarkdown("# Title\n\nHello **world** and _you_\n\n- one\n- two\n\n1. first");
    expect(b.map((x) => x.t)).toEqual(["h", "p", "ul", "ol"]);
    expect(parseInline("a **b** `c` [d](https://e.f) *g*").map((n) => n.t)).toEqual(["text", "b", "text", "code", "text", "link", "text", "i"]);
    expect(parseInline("snake_case_name").map((n) => n.t)).toEqual(["text"]);
  });

  it("never renders markup from the text", () => {
    const evil = 'Hi <img src=x onerror="alert(1)"> <script>alert(2)</script> [x](javascript:alert(3))';
    const { container } = render(<Markdown text={evil} />);
    expect(container.querySelector("img")).toBeNull();
    expect(container.querySelector("script")).toBeNull();
    expect(container.querySelector("a")).toBeNull();
    expect(container.textContent).toContain("<script>alert(2)</script>");
  });
});
