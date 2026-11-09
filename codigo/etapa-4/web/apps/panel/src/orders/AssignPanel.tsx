import { unwrap } from "@nugget/api";
import { Banner, Button, Field, Panel, Select, Skeleton, Stack } from "@nugget/ui";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { useApi } from "../api";
import { orderProblem, technicianOptions, type StaffOrder } from "./work";

/** AssignPanel lets the admin give an order awaiting review to a technician,
 * picked from the accounts Keycloak has with the role. The panel API checks
 * the role again. */
export function AssignPanel({ order }: { order: StaffOrder }) {
  const api = useApi();
  const client = useQueryClient();
  const technicians = useQuery({ queryKey: ["technicians"], queryFn: () => unwrap(api.GET("/panel/v1/technicians")), staleTime: 60_000 });
  const [picked, setPicked] = useState(order.assigned_technician ?? "");
  const assign = useMutation({
    mutationFn: (technician: string) =>
      unwrap(api.POST("/panel/v1/orders/{id}/assign", { params: { path: { id: order.id } }, body: { technician_id: technician } })),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ["order", order.id] });
      await client.invalidateQueries({ queryKey: ["orders"] });
    },
  });
  const options = technicianOptions(technicians.data?.technicians ?? []);
  const current = options.find((t) => t.value === order.assigned_technician);

  return (
    <Panel accent>
      <Panel.Header>
        <h2>Atribuição</h2>
      </Panel.Header>
      <Panel.Body>
        <Stack gap={3}>
          <p data-testid="assigned-technician">
            {order.assigned_technician === null ? "Nenhum técnico atribuído." : `Atribuído a ${current?.label ?? order.assigned_technician}.`}
          </p>
          {technicians.isPending ? (
            <Skeleton lines={1} />
          ) : technicians.isError ? (
            <Banner tone="danger">Não foi possível carregar os técnicos.</Banner>
          ) : (
            <Field label="Técnico">
              {(c) => (
                <Select {...c} value={picked} onChange={(e) => { setPicked(e.target.value); }}>
                  <option value="" disabled>
                    Escolha um técnico
                  </option>
                  {options.map((t) => (
                    <option key={t.value} value={t.value}>
                      {t.label}
                    </option>
                  ))}
                </Select>
              )}
            </Field>
          )}
          {assign.isError && <Banner tone="danger">{orderProblem(assign.error)}</Banner>}
        </Stack>
      </Panel.Body>
      <Panel.Footer>
        <Button
          variant="primary"
          disabled={picked === "" || picked === order.assigned_technician || assign.isPending}
          onClick={() => { assign.mutate(picked); }}
        >
          Atribuir
        </Button>
      </Panel.Footer>
    </Panel>
  );
}
