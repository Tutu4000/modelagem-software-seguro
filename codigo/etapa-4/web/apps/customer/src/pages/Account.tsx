import { unwrap } from "@nugget/api";
import { useSession } from "@nugget/auth";
import { Grid, Panel, Skeleton, Stack } from "@nugget/ui";
import { useQuery } from "@tanstack/react-query";

import { useApi } from "../api";

const time = new Intl.DateTimeFormat("pt-BR", { dateStyle: "short", timeStyle: "short" });

export function Account() {
  const { state } = useSession();
  const api = useApi();
  const catalog = useQuery({ queryKey: ["catalog"], queryFn: () => unwrap(api.GET("/api/v1/catalog")) });
  if (state.status !== "authenticated") {
    return null;
  }
  const { claims } = state;
  return (
    <Stack gap={4}>
      <h1>Minha conta</h1>
      <Grid columns={2}>
        <Panel>
          <Panel.Header>
            <h2>Cadastro</h2>
          </Panel.Header>
          <Panel.Body>
            <dl>
              <dt>Usuário</dt>
              <dd>{claims.username}</dd>
              <dt>E-mail</dt>
              <dd>{claims.email ?? "—"}</dd>
            </dl>
          </Panel.Body>
        </Panel>
        <Panel>
          <Panel.Header>
            <h2>Sessão</h2>
          </Panel.Header>
          <Panel.Body>
            <dl>
              <dt>Login em</dt>
              <dd className="mono">{time.format(claims.authTime)}</dd>
              <dt>Acesso renovado até</dt>
              <dd className="mono">{time.format(claims.expiresAt)}</dd>
              <dt>Tabela de preço ativa</dt>
              <dd className="mono" data-testid="catalog-version">
                {catalog.data === undefined ? <Skeleton /> : `versão ${String(catalog.data.version)}`}
              </dd>
            </dl>
          </Panel.Body>
        </Panel>
      </Grid>
    </Stack>
  );
}
