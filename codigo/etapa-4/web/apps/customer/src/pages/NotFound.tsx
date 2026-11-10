import { Panel, Stack } from "@nugget/ui";
import { Link } from "react-router";

export function NotFound() {
  return (
    <Panel>
      <Panel.Body>
        <Stack gap={3}>
          <h1>Página não encontrada</h1>
          <p>O endereço não existe ou ainda não está disponível.</p>
          <Link to="/">Voltar ao início</Link>
        </Stack>
      </Panel.Body>
    </Panel>
  );
}
