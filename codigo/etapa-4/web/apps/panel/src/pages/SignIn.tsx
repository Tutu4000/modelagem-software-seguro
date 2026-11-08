import { Button, Grid, Panel, ShieldIcon, Stack } from "@nugget/ui";

export function SignIn() {
  return (
    <Grid columns="sidebar">
      <Panel accent>
        <Panel.Header>
          <h1>Entre com sua conta da equipe</h1>
          <ShieldIcon />
        </Panel.Header>
        <Panel.Body>
          <Stack gap={3}>
            <p>O painel aceita só o login com senha e código TOTP. As operações críticas pedem um login recente e uma justificativa.</p>
            <p>Técnicos veem os pedidos atribuídos a eles; suporte e administração veem a fila completa.</p>
          </Stack>
        </Panel.Body>
        <Panel.Footer>
          <Button variant="primary" disabled>
            Entrar com MFA
          </Button>
        </Panel.Footer>
      </Panel>
      <Panel>
        <Panel.Header>
          <h2>Lembre</h2>
        </Panel.Header>
        <Panel.Body>
          <Stack gap={2}>
            <p>Não compartilhe a sessão nem o código TOTP.</p>
            <p>Saia do painel quando terminar.</p>
          </Stack>
        </Panel.Body>
      </Panel>
    </Grid>
  );
}
