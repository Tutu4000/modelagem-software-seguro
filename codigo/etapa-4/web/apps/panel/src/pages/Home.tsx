import { useSession } from "@nugget/auth";
import { Banner, Button, Grid, Panel, ShieldIcon, Skeleton, Stack } from "@nugget/ui";
import { useState } from "react";

import { useMe } from "../api";
import { roleLabel } from "../roles";

const time = new Intl.DateTimeFormat("pt-BR", { dateStyle: "short", timeStyle: "medium" });

export function Home() {
  const session = useSession();
  const me = useMe();
  const [stepUpFailed, setStepUpFailed] = useState(false);

  const confirm = async () => {
    setStepUpFailed(false);
    try {
      await session.stepUp();
      await me.refetch();
    } catch {
      setStepUpFailed(true);
    }
  };

  return (
    <Stack gap={4}>
      <h1>Painel interno</h1>
      <Grid columns="sidebar">
        <Panel>
          <Panel.Header>
            <h2>Sua sessão</h2>
            <ShieldIcon />
          </Panel.Header>
          <Panel.Body>
            {me.data === undefined ? (
              <Skeleton lines={3} />
            ) : (
              <dl>
                <dt>Papel</dt>
                <dd data-testid="role">{roleLabel[me.data.role]}</dd>
                <dt>Conta</dt>
                <dd className="mono">{me.data.id}</dd>
                <dt>Login com MFA em</dt>
                <dd className="mono">{time.format(new Date(me.data.authenticated_at))}</dd>
                <dt>Operações críticas liberadas até</dt>
                <dd className="mono" data-testid="step-up-until">
                  {time.format(new Date(me.data.step_up_needed_at))}
                </dd>
              </dl>
            )}
          </Panel.Body>
        </Panel>
        <Panel accent>
          <Panel.Header>
            <h2>Operações críticas</h2>
          </Panel.Header>
          <Panel.Body>
            <Stack gap={3}>
              <p>Revogar usuários, abrir projetos, aprovar preços e trocar endereços pedem um login com MFA de no máximo 5 minutos e uma justificativa, que fica na auditoria.</p>
              {stepUpFailed && <Banner tone="warning">A confirmação não foi concluída.</Banner>}
            </Stack>
          </Panel.Body>
          <Panel.Footer>
            <Button variant="primary" onClick={() => void confirm()}>
              Confirmar identidade agora
            </Button>
          </Panel.Footer>
        </Panel>
      </Grid>
    </Stack>
  );
}
