import { ApiError } from "@nugget/api";
import type { StlProblem } from "@nugget/viewer";

/** What the customer reads when the file is refused in the browser. */
export const stlProblem: Record<StlProblem, string> = {
  empty: "O arquivo está vazio.",
  too_large: "O arquivo passa de 100 MB.",
  not_binary: "O arquivo não é um STL binário. Exporte o modelo como STL binário e tente de novo.",
  truncated: "O arquivo está incompleto ou corrompido.",
  too_many_triangles: "O modelo passa de 2 milhões de triângulos. Simplifique a malha e tente de novo.",
};

/** uploadMessage is the answer of the API to the upload, in words. Limits,
 * outages and partners go to the page banner instead. */
export function uploadMessage(e: unknown): string {
  if (!(e instanceof ApiError)) {
    return "O envio não foi concluído. Confira a conexão e tente de novo.";
  }
  switch (e.code) {
    case "invalid_stl":
      return "O servidor recusou o arquivo: não é um STL binário válido.";
    case "too_many_triangles":
      return stlProblem.too_many_triangles;
    case "file_too_large":
      return stlProblem.too_large;
    case "rate_limited":
      return "Muitos envios seguidos. Aguarde um pouco.";
    default:
      return "O servidor recusou o arquivo.";
  }
}

/** quoteMessage explains why a quote or its submission did not go through. */
export function quoteMessage(e: unknown): string | undefined {
  if (!(e instanceof ApiError)) {
    return undefined;
  }
  switch (e.code) {
    case "quote_expired":
      return "O orçamento venceu (vale 24 horas). Calcule de novo.";
    case "conflict":
      return "O arquivo ainda não terminou de ser processado, ou o pedido já foi enviado.";
    case "invalid_request":
      return "Confira as opções e o endereço.";
    case "forbidden":
      return "Este arquivo não é da sua conta.";
    default:
      return undefined;
  }
}
