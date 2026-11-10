import { AddressFields, colorLabel, finishingLabel, qualityLabel, type AddressErrors } from "@nugget/domain";
import { ChipGroup, Field, money, Panel, Stack, Stepper } from "@nugget/ui";

import type { Address, Choice, Rules } from "./types";

export interface ConfigFormProps {
  rules: Rules;
  choice: Choice;
  onChoice: (c: Choice) => void;
  address: Address;
  onAddress: (a: Address) => void;
  /** Shown once the customer tried to quote. */
  errors: AddressErrors;
  disabled: boolean;
}

/** ConfigForm is the order configuration, built from the active price table:
 * the colors follow the material, and nothing here computes a price. */
export function ConfigForm({ rules, choice, onChoice, address, onAddress, errors, disabled }: ConfigFormProps) {
  const material = rules.materials[choice.material];
  const set = (patch: Partial<Choice>) => { onChoice({ ...choice, ...patch }); };
  return (
    <Panel>
      <Panel.Header>
        <h2>Configuração</h2>
      </Panel.Header>
      <Panel.Body>
        <Stack gap={5}>
          <ChipGroup
            legend="Material"
            disabled={disabled}
            value={choice.material}
            options={Object.entries(rules.materials).map(([key, m]) => ({ value: key, label: m.name, detail: `${money(m.cents_per_cm3)}/cm³` }))}
            onChange={(key) => {
              const next = rules.materials[key];
              set({ material: key, color: next?.colors.includes(choice.color) === true ? choice.color : (next?.colors[0] ?? "") });
            }}
          />
          <ChipGroup
            legend="Cor"
            disabled={disabled}
            value={choice.color}
            options={(material?.colors ?? []).map((c) => ({ value: c, label: colorLabel(c) }))}
            onChange={(color) => { set({ color }); }}
          />
          <ChipGroup
            legend="Qualidade"
            disabled={disabled}
            value={choice.quality}
            options={Object.keys(rules.qualities).map((q) => ({ value: q, label: qualityLabel(q) }))}
            onChange={(quality) => { set({ quality }); }}
          />
          <ChipGroup
            legend="Acabamento"
            disabled={disabled}
            value={choice.finishing}
            options={Object.entries(rules.finishings).map(([f, cents]) => ({
              value: f,
              label: finishingLabel(f),
              detail: cents > 0 ? `+ ${money(cents)}/un.` : undefined,
            }))}
            onChange={(finishing) => { set({ finishing }); }}
          />
          <Field
            label="Quantidade"
            hint={rules.discounts.length > 0 ? `Desconto a partir de ${rules.discounts.map((d) => `${String(d.min_quantity)} un. (${String(d.percent_off)}%)`).join(", ")}` : undefined}
          >
            {(c) => <Stepper {...c} value={choice.quantity} min={1} max={1000} onChange={(quantity) => { set({ quantity }); }} />}
          </Field>

          <Stack gap={3}>
            <h3>Entrega</h3>
            <AddressFields address={address} onChange={onAddress} errors={errors} disabled={disabled} />
          </Stack>
        </Stack>
      </Panel.Body>
    </Panel>
  );
}
