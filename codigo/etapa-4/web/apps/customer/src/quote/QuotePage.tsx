import { unwrap } from "@nugget/api";
import { Banner, Button, money, Panel, Skeleton, Stack, Steps } from "@nugget/ui";
import { useMutation, useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { Link, useSearchParams } from "react-router";

import { useApi } from "../api";
import { emptyAddress, validateAddress } from "./address";
import { ConfigForm } from "./ConfigForm";
import { FileStep, useFileStatus } from "./FileStep";
import { quoteMessage } from "./messages";
import styles from "./Quote.module.css";
import { QuotePanel } from "./QuotePanel";
import type { Address, Choice, Order, Rules } from "./types";

function firstKey(record: Record<string, unknown>, preferred: string): string {
  return preferred in record ? preferred : (Object.keys(record)[0] ?? "");
}

function defaults(rules: Rules): Choice {
  const material = firstKey(rules.materials, "pla");
  return {
    material,
    color: rules.materials[material]?.colors[0] ?? "",
    quality: firstKey(rules.qualities, "standard"),
    finishing: firstKey(rules.finishings, "none"),
    quantity: 1,
  };
}

/** QuotePage is the quote flow: the file, the configuration and the server's
 * price, side by side on a desktop and one after the other on a phone. The
 * uploaded file stays in the URL, so a reload resumes from it. */
export function QuotePage() {
  const api = useApi();
  const [params, setParams] = useSearchParams();
  const fileId = params.get("arquivo") ?? undefined;
  const status = useFileStatus(fileId);
  const catalog = useQuery({ queryKey: ["catalog"], queryFn: () => unwrap(api.GET("/api/v1/catalog")) });

  const [choice, setChoice] = useState<Choice | undefined>();
  const [address, setAddress] = useState<Address>(emptyAddress);
  const [tried, setTried] = useState(false);
  const [order, setOrder] = useState<Order | undefined>();
  const [stale, setStale] = useState(false);
  const [sent, setSent] = useState<Order | undefined>();

  const rules = catalog.data?.rules;
  const config = choice ?? (rules === undefined ? undefined : defaults(rules));
  const addressErrors = validateAddress(address);
  const valid = Object.keys(addressErrors).length === 0;

  const quote = useMutation({
    mutationFn: (c: Choice) =>
      unwrap(api.POST("/api/v1/orders", { body: { file_id: fileId ?? "", ...c, shipping: { ...address, postal_code: address.postal_code } } })),
    onSuccess: (o) => {
      setOrder(o);
      setStale(false);
    },
  });
  const submit = useMutation({
    mutationFn: (id: string) => unwrap(api.POST("/api/v1/orders/{id}/submit", { params: { path: { id } } })),
    onSuccess: setSent,
  });

  const quoted = order !== undefined && !stale;
  const canQuote = status.data?.status === "done" && config !== undefined;
  const busy = quote.isPending || submit.isPending;
  const askQuote = () => {
    setTried(true);
    if (valid && config !== undefined) {
      quote.mutate(config);
    }
  };
  const send = () => {
    if (order !== undefined) {
      submit.mutate(order.id);
    }
  };

  const changed = () => {
    if (order !== undefined) {
      setStale(true);
    }
  };

  if (sent !== undefined) {
    return (
      <Panel accent>
        <Panel.Header>
          <h1>Pedido enviado para a revisão técnica</h1>
        </Panel.Header>
        <Panel.Body>
          <Stack gap={3}>
            <p>
              Pedido <code data-testid="order-id">{sent.id.slice(0, 8)}</code>. Um técnico confere a manufaturabilidade e registra o laudo. Você recebe um e-mail
              quando a revisão terminar e, se aprovada, paga pela página do pedido.
            </p>
          </Stack>
        </Panel.Body>
        <Panel.Footer>
          <Link to={`/pedidos/${sent.id}`}>Acompanhar o pedido</Link>
          <Link to="/orcar" onClick={() => {
            setSent(undefined);
            setOrder(undefined);
            setChoice(undefined);
          }}>
            Novo orçamento
          </Link>
        </Panel.Footer>
      </Panel>
    );
  }

  const step = fileId === undefined || status.data?.status !== "done" ? 0 : order === undefined || stale ? 1 : 2;
  return (
    <Stack gap={4}>
      <h1>Orçar uma peça</h1>
      <Steps label="Etapas do orçamento" steps={["Arquivo", "Configuração", "Orçamento"]} current={step} />
      <div className={styles.layout}>
        <FileStep
          fileId={fileId}
          status={status.data}
          onReset={() => {
            setOrder(undefined);
            setParams({});
          }}
          onUploaded={(id) => {
            setOrder(undefined);
            setParams({ arquivo: id });
          }}
        />
        {rules === undefined || config === undefined ? (
          <Panel>
            <Panel.Body>
              {catalog.isError ? <Banner tone="danger">Não foi possível carregar o catálogo.</Banner> : <Skeleton lines={8} />}
            </Panel.Body>
          </Panel>
        ) : (
          <ConfigForm
            rules={rules}
            choice={config}
            onChoice={(c) => {
              setChoice(c);
              changed();
            }}
            address={address}
            onAddress={(a) => {
              setAddress(a);
              changed();
            }}
            errors={tried ? addressErrors : {}}
            disabled={busy}
          />
        )}
        <div className={styles.summary}>
          <QuotePanel
            order={order}
            stale={stale}
            canQuote={canQuote}
            busy={busy}
            problem={quoteMessage(quote.error ?? submit.error)}
            onQuote={askQuote}
            onSubmit={send}
          />
        </div>
      </div>
      {/* The phone's shortcut to the summary's actions, which stay the
          accessible ones. */}
      <div className={styles.bar} aria-hidden="true">
        <span className={styles.barTotal}>{quoted ? money(order.total_cents) : "—"}</span>
        <Button variant="primary" tabIndex={-1} disabled={busy || (!quoted && !canQuote)} onClick={quoted ? send : askQuote}>
          {quoted ? "Enviar para revisão" : "Calcular"}
        </Button>
      </div>
      {fileId !== undefined && status.data === undefined && status.isError && (
        <Banner tone="danger" action={<Button onClick={() => { setParams({}); }}>Começar de novo</Button>}>
          Não encontramos este arquivo na sua conta.
        </Banner>
      )}
    </Stack>
  );
}
