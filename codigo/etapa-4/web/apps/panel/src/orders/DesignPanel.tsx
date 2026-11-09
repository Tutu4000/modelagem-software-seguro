import { unwrap } from "@nugget/api";
import { Banner, Button, Panel, Skeleton, Stack } from "@nugget/ui";
import { parseInWorker, type Mesh } from "@nugget/viewer";
import { lazy, Suspense, useState } from "react";

import { useApi } from "../api";
import { CriticalActionDialog } from "../CriticalActionDialog";

const StlViewer = lazy(() => import("@nugget/viewer/StlViewer").then((m) => ({ default: m.StlViewer })));

/** DesignPanel opens the customer's design for the assigned technician: a
 * critical operation (CA08), with a justification and a recent login. The
 * signed URL is read at once and dropped; the panel offers no download link,
 * only the 3D view. */
export function DesignPanel({ orderId }: { orderId: string }) {
  const api = useApi();
  const [asking, setAsking] = useState(false);
  const [mesh, setMesh] = useState<Mesh | undefined>();
  const [problem, setProblem] = useState<string | undefined>();

  const open = async (justification: string) => {
    const { url } = await unwrap(api.POST("/panel/v1/orders/{id}/file", { params: { path: { id: orderId } }, body: { justification } }));
    setProblem(undefined);
    try {
      const res = await fetch(url, { mode: "cors", credentials: "omit" });
      if (!res.ok) {
        throw new Error(`storage answered ${String(res.status)}`);
      }
      const parsed = await parseInWorker(await res.blob());
      if (parsed.ok) {
        setMesh(parsed.mesh);
      } else {
        setProblem("O arquivo não pôde ser lido como STL binário.");
      }
    } catch {
      setProblem("Não foi possível baixar o projeto. Abra de novo para pedir outro link.");
    }
  };

  return (
    <Panel>
      <Panel.Header>
        <h2>Projeto</h2>
      </Panel.Header>
      <Panel.Body>
        <Stack gap={3}>
          {mesh === undefined ? (
            <p>O projeto do cliente só abre com uma justificativa, que fica na auditoria, e um login recente com MFA.</p>
          ) : (
            <Suspense fallback={<Skeleton lines={6} />}>
              <StlViewer mesh={mesh} label="Projeto do cliente" />
            </Suspense>
          )}
          {problem !== undefined && <Banner tone="danger">{problem}</Banner>}
        </Stack>
      </Panel.Body>
      <Panel.Footer>
        <Button variant={mesh === undefined ? "primary" : "ghost"} onClick={() => { setAsking(true); }}>
          {mesh === undefined ? "Abrir projeto" : "Abrir de novo"}
        </Button>
      </Panel.Footer>
      <CriticalActionDialog
        open={asking}
        onClose={() => { setAsking(false); }}
        title="Abrir o projeto do cliente"
        confirmLabel="Abrir projeto"
        onConfirm={open}
      >
        <p>O link vale 5 minutos e só mostra a peça aqui no painel. Explique por que precisa ver o projeto.</p>
      </CriticalActionDialog>
    </Panel>
  );
}
