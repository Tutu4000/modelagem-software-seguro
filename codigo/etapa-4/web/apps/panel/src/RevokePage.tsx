import { ApiError, unwrap } from "@nugget/api";
import { Banner, Button, Field, KeyValue, Panel, Stack, TextInput } from "@nugget/ui";
import { useState } from "react";
import { useSearchParams } from "react-router";

import { useApi, useMe } from "./api";
import { CriticalActionDialog } from "./CriticalActionDialog";

const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export function isUuid(text: string): boolean {
  return uuid.test(text.trim());
}

type Outcome = { kind: "revoked" } | { kind: "idp_failed" };

/** RevokePage is the incident response (E6.4): it cuts every token of the
 * account at once, disables it and ends its sessions. Critical: a recent
 * login and a justification. */
export function RevokePage() {
  const api = useApi();
  const me = useMe();
  const [search] = useSearchParams();
  const [user, setUser] = useState(search.get("usuario") ?? "");
  const [tried, setTried] = useState(false);
  const [asking, setAsking] = useState(false);
  const [outcome, setOutcome] = useState<Outcome | undefined>();
  const id = user.trim().toLowerCase();
  const self = me.data?.id === id;
  const error = !tried ? undefined : !isUuid(id) ? "Use o ID da conta (UUID)." : self ? "Você não pode revogar a própria conta." : undefined;

  const revoke = async (justification: string) => {
    setOutcome(undefined);
    try {
      await unwrap(api.POST("/panel/v1/users/{id}/revoke", { params: { path: { id } }, body: { justification } }));
      setOutcome({ kind: "revoked" });
    } catch (e) {
      if (e instanceof ApiError && e.status === 502) {
        // The tokens are already cut; the admin must retry for the account.
        setOutcome({ kind: "idp_failed" });
        return;
      }
      throw e;
    }
  };

  return (
    <Stack gap={4}>
      <h1>Revogar acesso</h1>
      <Panel accent>
        <Panel.Body>
          <Stack gap={3}>
            <p>Use numa suspeita de conta comprometida. A revogação vale na hora para todas as réplicas da API e do painel:</p>
            <KeyValue
              pairs={[
                { label: "1. Tokens", value: "Todos os tokens da conta deixam de valer." },
                { label: "2. Conta", value: "Desativada no Keycloak: sem login novo nem renovação." },
                { label: "3. Sessões", value: "Encerradas em todos os aplicativos." },
              ]}
            />
            <Field label="ID da conta" error={error}>
              {(c) => <TextInput {...c} className="mono" value={user} onChange={(e) => { setUser(e.target.value); setOutcome(undefined); }} />}
            </Field>
            {outcome?.kind === "revoked" && (
              <Banner tone="info">
                <span data-testid="revoke-result">Acesso revogado: tokens cortados, conta desativada e sessões encerradas.</span>
              </Banner>
            )}
            {outcome?.kind === "idp_failed" && (
              <Banner tone="warning">Os tokens já foram cortados, mas o Keycloak não respondeu. Revogue de novo para desativar a conta.</Banner>
            )}
          </Stack>
        </Panel.Body>
        <Panel.Footer>
          <Button
            variant="primary"
            onClick={() => {
              setTried(true);
              if (isUuid(id) && !self) {
                setAsking(true);
              }
            }}
          >
            Revogar
          </Button>
        </Panel.Footer>
      </Panel>
      <CriticalActionDialog open={asking} onClose={() => { setAsking(false); }} title="Revogar o acesso da conta" confirmLabel="Revogar agora" onConfirm={revoke}>
        <p>
          Conta <span className="mono">{id}</span>. Para devolver o acesso, a conta precisa ser reativada no Keycloak.
        </p>
      </CriticalActionDialog>
    </Stack>
  );
}
