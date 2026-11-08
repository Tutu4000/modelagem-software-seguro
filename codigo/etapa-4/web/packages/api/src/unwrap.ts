import { ApiError, toApiError } from "./problem";

interface Result<D> {
  data?: D;
  error?: unknown;
  response: Response;
}

/** unwrap turns an openapi-fetch result into its data, or throws an ApiError
 * with the problem code: the shape the query hooks expect. */
export async function unwrap<D>(call: Promise<Result<D>>): Promise<D> {
  const { data, error, response } = await call;
  if (!response.ok) {
    throw toApiError(response, error);
  }
  if (data === undefined) {
    throw new ApiError(response.status, "empty_body");
  }
  return data;
}

/** isStepUp: a critical operation needs a recent login (RFC 9470, E6.4). */
export function isStepUp(e: unknown): boolean {
  return e instanceof ApiError && e.status === 401 && e.code === "step_up_required";
}

/** isUnauthenticated: the token is gone or revoked, not a step-up. */
export function isUnauthenticated(e: unknown): boolean {
  return e instanceof ApiError && e.status === 401 && e.code !== "step_up_required";
}

export interface ErrorNotice {
  tone: "warning" | "danger";
  text: string;
}

/** notice says, in Portuguese, what the user should know about an error that
 * is not about the form: limits, outages and partners. Undefined otherwise:
 * the screen handles it. */
export function notice(e: unknown): ErrorNotice | undefined {
  if (!(e instanceof ApiError)) {
    return e instanceof TypeError ? { tone: "danger", text: "Sem conexão com o servidor. Confira a rede e tente de novo." } : undefined;
  }
  switch (true) {
    case e.status === 429:
      return {
        tone: "warning",
        text: e.retryAfter === undefined ? "Muitas requisições. Aguarde um pouco e tente de novo." : `Muitas requisições. Tente de novo em ${String(e.retryAfter)} s.`,
      };
    case e.code === "partner_unavailable":
      return { tone: "warning", text: "Um parceiro (pagamento ou transportadora) está fora do ar. Nada foi alterado; tente de novo em instantes." };
    case e.status === 503:
      return { tone: "danger", text: "Serviço indisponível no momento. Tente de novo em instantes." };
    case e.status >= 500:
      return { tone: "danger", text: "Erro inesperado no servidor." };
    default:
      return undefined;
  }
}
