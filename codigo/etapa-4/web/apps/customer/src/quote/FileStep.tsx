import { unwrap, uploadStl } from "@nugget/api";
import { useSession } from "@nugget/auth";
import { Banner, FileDrop, Panel, Progress, Skeleton, Spinner, Stack, StatusTag, type Tone } from "@nugget/ui";
import { dimensions, maxBytes, parseInWorker, type Mesh } from "@nugget/viewer";
import { useQuery } from "@tanstack/react-query";
import { lazy, Suspense, useState } from "react";

import { useApi } from "../api";
import { stlProblem, uploadMessage } from "./messages";
import type { FileStatus } from "./types";

const StlViewer = lazy(() => import("@nugget/viewer/StlViewer").then((m) => ({ default: m.StlViewer })));

const statusView: Record<FileStatus["status"], { tone: Tone; label: string }> = {
  waiting: { tone: "neutral", label: "Na fila" },
  processing: { tone: "progress", label: "Processando" },
  done: { tone: "success", label: "Pronto" },
  failed: { tone: "danger", label: "Falhou" },
  purged: { tone: "neutral", label: "Apagado" },
};

export interface FileStepProps {
  fileId: string | undefined;
  onUploaded: (id: string) => void;
  onReset: () => void;
  /** Reports the server's status, which the quote waits for. */
  status: FileStatus | undefined;
}

/** useFileStatus polls the processing every second until it ends. */
export function useFileStatus(fileId: string | undefined) {
  const api = useApi();
  return useQuery({
    queryKey: ["file", fileId],
    enabled: fileId !== undefined,
    queryFn: () => unwrap(api.GET("/api/v1/files/{id}", { params: { path: { id: fileId ?? "" } } })),
    refetchInterval: (q) => {
      const s = q.state.data?.status;
      return s === "waiting" || s === "processing" ? 1000 : false;
    },
  });
}

/** FileStep picks the STL, shows it at once (parsed here, in a worker),
 * uploads it with progress and follows the processing. After a reload the
 * preview comes from the file already uploaded, through its signed URL. */
export function FileStep({ fileId, onUploaded, onReset, status }: FileStepProps) {
  const session = useSession();
  const api = useApi();
  const [mesh, setMesh] = useState<Mesh | undefined>();
  const [name, setName] = useState<string | undefined>();
  const [problem, setProblem] = useState<string | undefined>();
  const [progress, setProgress] = useState<number | undefined>();

  const remote = useQuery({
    queryKey: ["preview", fileId],
    enabled: fileId !== undefined && mesh === undefined && status?.status === "done",
    staleTime: Infinity,
    queryFn: async () => {
      const { url } = await unwrap(api.GET("/api/v1/files/{id}/download", { params: { path: { id: fileId ?? "" } } }));
      const blob = await (await fetch(url, { mode: "cors", credentials: "omit" })).blob();
      const parsed = await parseInWorker(blob);
      return parsed.ok ? parsed.mesh : undefined;
    },
  });
  const shown = mesh ?? remote.data;

  const pick = async (file: File) => {
    setProblem(undefined);
    setMesh(undefined);
    setName(file.name);
    onReset();
    if (!file.name.toLowerCase().endsWith(".stl")) {
      setProblem("Escolha um arquivo .stl.");
      return;
    }
    if (file.size > maxBytes) {
      setProblem(stlProblem.too_large);
      return;
    }
    const parsed = await parseInWorker(file);
    if (!parsed.ok) {
      setProblem(stlProblem[parsed.problem]);
      return;
    }
    setMesh(parsed.mesh);
    setProgress(0);
    try {
      const accepted = await uploadStl(file, { token: session.token(), onProgress: setProgress });
      onUploaded(accepted.id);
    } catch (e) {
      setProblem(uploadMessage(e));
    } finally {
      setProgress(undefined);
    }
  };

  const view = status === undefined ? undefined : statusView[status.status];
  return (
    <Panel>
      <Panel.Header>
        <h2>Arquivo</h2>
        {view !== undefined && <StatusTag tone={view.tone}>{view.label}</StatusTag>}
      </Panel.Header>
      <Panel.Body>
        <Stack gap={3}>
          {shown !== undefined ? (
            <Suspense fallback={<Skeleton lines={6} />}>
              <StlViewer mesh={shown} label={name ?? "Peça enviada"} />
            </Suspense>
          ) : fileId !== undefined && status?.status === "done" ? (
            <Spinner label="Carregando a visualização…" />
          ) : null}
          {progress !== undefined && <Progress value={progress} label="Envio do arquivo" />}
          {problem !== undefined && <Banner tone="danger">{problem}</Banner>}
          {status?.status === "waiting" && (
            <Banner tone="info">Você já tem 3 arquivos em processamento. Este começa assim que um deles terminar.</Banner>
          )}
          {status?.status === "failed" && <Banner tone="danger">O processamento falhou. Confira o modelo e envie de novo.</Banner>}
          {status?.status === "done" && status.result !== undefined && (
            <dl>
              <dt>Medidas (servidor)</dt>
              <dd className="mono" data-testid="server-dimensions">
                {dimensions([status.result.bbox_mm[0] ?? 0, status.result.bbox_mm[1] ?? 0, status.result.bbox_mm[2] ?? 0])}
              </dd>
              <dt>Volume</dt>
              <dd className="mono">{(status.result.volume_mm3 / 1000).toLocaleString("pt-BR", { maximumFractionDigits: 2 })} cm³</dd>
            </dl>
          )}
          <FileDrop
            accept=".stl,model/stl"
            disabled={progress !== undefined}
            onFile={(f) => void pick(f)}
            hint={fileId === undefined ? "STL binário, até 100 MB e 2 milhões de triângulos." : "Escolher outro arquivo começa um orçamento novo."}
          />
        </Stack>
      </Panel.Body>
    </Panel>
  );
}
