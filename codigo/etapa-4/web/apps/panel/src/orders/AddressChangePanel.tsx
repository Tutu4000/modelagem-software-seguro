import { ApiError, unwrap } from "@nugget/api";
import { AddressFields, emptyAddress, validateAddress, type Address } from "@nugget/domain";
import { Banner, Button, Panel, Stack } from "@nugget/ui";
import { useState } from "react";

import { useApi } from "../api";
import { CriticalActionDialog } from "../CriticalActionDialog";

const until = new Intl.DateTimeFormat("pt-BR", { timeStyle: "short" });

export interface AddressChangePanelProps {
  orderId: string;
  /** The ticket the support came from: the change is tied to it in the
   * audit trail. */
  ticketId: string | undefined;
}

/** AddressChangePanel proposes a new delivery address (C21), as a critical
 * operation. The support never changes it: the customer confirms with the
 * code sent to the registered e-mail, which the panel never sees. */
export function AddressChangePanel({ orderId, ticketId }: AddressChangePanelProps) {
  const api = useApi();
  const [address, setAddress] = useState<Address>(emptyAddress);
  const [tried, setTried] = useState(false);
  const [asking, setAsking] = useState(false);
  const [sent, setSent] = useState<string | undefined>();
  const [problem, setProblem] = useState<string | undefined>();
  const errors = tried ? validateAddress(address) : {};

  const propose = async (justification: string) => {
    setProblem(undefined);
    try {
      const res = await unwrap(
        api.POST("/panel/v1/orders/{id}/address-change", {
          params: { path: { id: orderId } },
          body: { address, justification, ...(ticketId === undefined ? {} : { ticket_id: ticketId }) },
        }),
      );
      setSent(res.expires_at);
    } catch (e) {
      if (e instanceof ApiError && e.status === 409) {
        // Not a failure of the dialog: the order cannot take a change.
        setProblem("O pedido já foi despachado ou não tem e-mail cadastrado. Não há troca possível.");
        return;
      }
      throw e;
    }
  };

  return (
    <Panel>
      <Panel.Header>
        <h2>Troca de endereço</h2>
      </Panel.Header>
      <Panel.Body>
        <Stack gap={3}>
          <p>O cliente confirma a troca com o código enviado ao e-mail cadastrado. Sem o código, o endereço não muda.</p>
          <AddressFields address={address} onChange={setAddress} errors={errors} />
          {sent !== undefined && (
            <Banner tone="info">
              <span data-testid="address-change-sent">Código enviado ao cliente. Vale até {until.format(new Date(sent))}.</span>
            </Banner>
          )}
          {problem !== undefined && <Banner tone="danger">{problem}</Banner>}
        </Stack>
      </Panel.Body>
      <Panel.Footer>
        <Button
          variant="primary"
          onClick={() => {
            setTried(true);
            if (Object.keys(validateAddress(address)).length === 0) {
              setAsking(true);
            }
          }}
        >
          Propor troca
        </Button>
      </Panel.Footer>
      <CriticalActionDialog open={asking} onClose={() => { setAsking(false); }} title="Propor troca de endereço" confirmLabel="Enviar código ao cliente" onConfirm={propose}>
        <p>
          Novo endereço: {address.recipient}, {address.street}, {address.number}, {address.city} - {address.state}, {address.postal_code}. Uma proposta nova substitui a
          pendente.
        </p>
      </CriticalActionDialog>
    </Panel>
  );
}
