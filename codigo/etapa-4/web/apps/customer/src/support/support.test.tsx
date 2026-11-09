import { Conversation } from "@nugget/ui";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { cleanText, maxMessage, ticketStatus } from "./text";

describe("cleanText", () => {
  it("trims and normalizes the line breaks, like the API", () => {
    expect(cleanText("  linha 1\r\nlinha 2  ", maxMessage)).toEqual({ ok: true, text: "linha 1\nlinha 2", length: 15 });
  });

  it("refuses empty, too long and control characters", () => {
    expect(cleanText("   ", maxMessage).ok).toBe(false);
    expect(cleanText("x".repeat(maxMessage + 1), maxMessage).ok).toBe(false);
    expect(cleanText("sino \u0007 aqui", maxMessage).ok).toBe(false);
    expect(cleanText("esc \u001b[31m", maxMessage).ok).toBe(false);
  });

  it("counts code points, accepting accents and emoji", () => {
    expect(cleanText("👍".repeat(maxMessage), maxMessage).ok).toBe(true);
    expect(cleanText("ação", 4)).toMatchObject({ ok: true, length: 4 });
  });
});

describe("ticketStatus", () => {
  it("names the three statuses", () => {
    expect(ticketStatus("open").label).toBe("Aguardando o suporte");
    expect(ticketStatus("answered").label).toBe("Respondido");
    expect(ticketStatus("closed").label).toBe("Fechado");
  });
});

describe("Conversation", () => {
  it("shows markup as text, never as elements (stored XSS)", () => {
    const { container } = render(
      <Conversation label="Mensagens">
        <Conversation.Message side="other" author="Suporte" at="agora">
          {'<b>oi</b><img src=x onerror="alert(1)">'}
        </Conversation.Message>
      </Conversation>,
    );
    expect(screen.getByText('<b>oi</b><img src=x onerror="alert(1)">')).toBeInTheDocument();
    expect(container.querySelector("b")).toBeNull();
    expect(container.querySelector("img")).toBeNull();
  });
});
