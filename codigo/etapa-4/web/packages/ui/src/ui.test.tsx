import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it } from "vitest";

import { DataTable, Field, StatusTag, Stepper, TextInput } from "./index";

describe("Field", () => {
  it("ties the label, the hint and the error to the control", () => {
    render(
      <Field label="CEP" hint="Só números" error="CEP inválido">
        {(control) => <TextInput {...control} />}
      </Field>,
    );
    const input = screen.getByLabelText("CEP");
    expect(input).toHaveAttribute("aria-invalid", "true");
    expect(input).toHaveAccessibleDescription("Só números CEP inválido");
  });

  it("leaves a valid control alone", () => {
    render(<Field label="Cidade">{(control) => <TextInput {...control} />}</Field>);
    expect(screen.getByLabelText("Cidade")).not.toHaveAttribute("aria-invalid");
  });
});

interface Row {
  id: string;
  material: string;
  quantity: number;
}

describe("DataTable", () => {
  it("renders typed rows, with a label per cell for the phone cards", () => {
    const rows: Row[] = [{ id: "a", material: "PLA", quantity: 2 }];
    render(
      <DataTable
        caption="Pedidos"
        rows={rows}
        rowKey={(r) => r.id}
        columns={[
          { key: "material", header: "Material" },
          { key: "quantity", header: "Qtd.", align: "end", render: (r) => `${String(r.quantity)} un.` },
        ]}
      />,
    );
    expect(screen.getByRole("table", { name: "Pedidos" })).toBeInTheDocument();
    expect(screen.getByText("2 un.")).toHaveAttribute("data-label", "Qtd.");
  });

  it("says so when there is nothing", () => {
    render(<DataTable<Row> caption="Pedidos" rows={[]} rowKey={(r) => r.id} columns={[]} empty="Sem pedidos" />);
    expect(screen.getByText("Sem pedidos")).toBeInTheDocument();
  });
});

describe("StatusTag", () => {
  it("always carries the text", () => {
    render(<StatusTag tone="success">Entregue</StatusTag>);
    expect(screen.getByText("Entregue")).toBeVisible();
  });
});

function Quantity() {
  const [n, setN] = useState(1);
  return <Stepper value={n} min={1} max={3} onChange={setN} id="q" />;
}

describe("Stepper", () => {
  it("stays between min and max", async () => {
    const user = userEvent.setup();
    render(<Quantity />);
    const input = screen.getByRole("spinbutton");
    expect(screen.getByRole("button", { name: "Diminuir" })).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "Aumentar" }));
    await user.click(screen.getByRole("button", { name: "Aumentar" }));
    await user.click(screen.getByRole("button", { name: "Aumentar" }));
    expect(input).toHaveValue(3);
    expect(screen.getByRole("button", { name: "Aumentar" })).toBeDisabled();
    await user.clear(input);
    await user.type(input, "99");
    expect(input).toHaveValue(3);
  });
});
