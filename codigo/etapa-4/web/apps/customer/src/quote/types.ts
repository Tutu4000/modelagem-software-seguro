import type { CustomerComponents, CustomerPaths } from "@nugget/api";

export type Catalog = CustomerPaths["/api/v1/catalog"]["get"]["responses"][200]["content"]["application/json"];
export type Rules = CustomerComponents["schemas"]["PriceRules"];
export type Order = CustomerComponents["schemas"]["Order"];
export type Address = CustomerComponents["schemas"]["Address"];
export type FileStatus = CustomerComponents["schemas"]["FileStatus"];
export type QuoteRequest = CustomerPaths["/api/v1/orders"]["post"]["requestBody"]["content"]["application/json"];

/** The options of the order, what the customer configures. */
export interface Choice {
  material: string;
  color: string;
  quality: string;
  finishing: string;
  quantity: number;
}
