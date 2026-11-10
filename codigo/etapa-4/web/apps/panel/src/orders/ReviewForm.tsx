import { unwrap } from "@nugget/api";
import { codePoints } from "@nugget/domain";
import { Banner, Button, ChipGroup, Field, Panel, Stack, TextArea } from "@nugget/ui";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { useApi } from "../api";
import { maxReport, orderProblem, reportError } from "./work";

type Decision = "approved" | "rejected";

const decisions: { value: Decision; label: string }[] = [
  { value: "approved", label: "Aprovar" },
  { value: "rejected", label: "Reprovar" },
];

/** ReviewForm records the technician's decision and the DFM report (E6.3).
 * There is one review per order: once recorded, the page shows it instead. */
export function ReviewForm({ orderId }: { orderId: string }) {
  const api = useApi();
  const client = useQueryClient();
  const [decision, setDecision] = useState<Decision | undefined>();
  const [report, setReport] = useState("");
  const [touched, setTouched] = useState(false);
  const review = useMutation({
    mutationFn: (body: { decision: Decision; report: string }) =>
      unwrap(api.POST("/panel/v1/orders/{id}/review", { params: { path: { id: orderId } }, body })),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ["order", orderId] });
      await client.invalidateQueries({ queryKey: ["orders"] });
    },
  });
  const error = touched ? reportError(report) : undefined;

  const submit = () => {
    setTouched(true);
    if (decision === undefined || reportError(report) !== undefined) {
      return;
    }
    review.mutate({ decision, report: report.trim() });
  };

  return (
    <Panel accent>
      <Panel.Header>
        <h2>Revisão técnica</h2>
      </Panel.Header>
      <Panel.Body>
        <Stack gap={3}>
          <ChipGroup legend="Decisão" value={decision} options={decisions} onChange={setDecision} disabled={review.isPending} />
          {touched && decision === undefined && <Banner tone="warning">Escolha aprovar ou reprovar.</Banner>}
          <Field label="Laudo" hint={`${String(codePoints(report.trim()))}/${String(maxReport)} · o cliente lê este texto`} error={error}>
            {(c) => (
              <TextArea
                {...c}
                rows={6}
                value={report}
                disabled={review.isPending}
                onChange={(e) => { setReport(e.target.value); }}
                onBlur={() => { setTouched(true); }}
              />
            )}
          </Field>
          {review.isError && <Banner tone="danger">{orderProblem(review.error)}</Banner>}
        </Stack>
      </Panel.Body>
      <Panel.Footer>
        <Button variant="primary" disabled={review.isPending} onClick={submit}>
          Registrar revisão
        </Button>
      </Panel.Footer>
    </Panel>
  );
}
