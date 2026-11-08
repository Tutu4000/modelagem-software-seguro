import { Banner, Button } from "@nugget/ui";
import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";

import { notice, type ErrorNotice } from "./unwrap";

/** ErrorBanner shows the latest limit, outage or partner error of any query
 * or mutation, at the top of the page, until dismissed. */
export function ErrorBanner() {
  const client = useQueryClient();
  const [current, setCurrent] = useState<ErrorNotice | undefined>();
  useEffect(() => {
    const show = (error: unknown) => {
      const n = notice(error);
      if (n !== undefined) {
        setCurrent(n);
      }
    };
    const offQueries = client.getQueryCache().subscribe((e) => {
      if (e.type === "updated" && e.action.type === "error") {
        show(e.action.error);
      }
    });
    const offMutations = client.getMutationCache().subscribe((e) => {
      if (e.type === "updated" && e.action.type === "error") {
        show(e.action.error);
      }
    });
    return () => {
      offQueries();
      offMutations();
    };
  }, [client]);
  if (current === undefined) {
    return null;
  }
  return (
    <Banner tone={current.tone} action={<Button variant="ghost" onClick={() => { setCurrent(undefined); }}>Fechar</Button>}>
      {current.text}
    </Banner>
  );
}
