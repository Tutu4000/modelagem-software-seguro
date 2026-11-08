import { ChipGroup, Field, Grid, money, Panel, Select, Stack, Stepper, TextInput } from "@nugget/ui";

import { maskPostalCode, states, type AddressErrors } from "./address";
import type { Address, Choice, Rules } from "./types";

const qualityLabel: Record<string, string> = { draft: "Rascunho", standard: "Padrão", fine: "Fina" };
const finishingLabel: Record<string, string> = { none: "Sem acabamento", sanded: "Lixado", painted: "Pintado" };
const colorLabel: Record<string, string> = { black: "Preto", white: "Branco", gray: "Cinza", red: "Vermelho", blue: "Azul", clear: "Transparente" };

const label = (names: Record<string, string>, key: string) => names[key] ?? key;

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
  const setAddress = (patch: Partial<Address>) => { onAddress({ ...address, ...patch }); };
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
            options={(material?.colors ?? []).map((c) => ({ value: c, label: label(colorLabel, c) }))}
            onChange={(color) => { set({ color }); }}
          />
          <ChipGroup
            legend="Qualidade"
            disabled={disabled}
            value={choice.quality}
            options={Object.keys(rules.qualities).map((q) => ({ value: q, label: label(qualityLabel, q) }))}
            onChange={(quality) => { set({ quality }); }}
          />
          <ChipGroup
            legend="Acabamento"
            disabled={disabled}
            value={choice.finishing}
            options={Object.entries(rules.finishings).map(([f, cents]) => ({
              value: f,
              label: label(finishingLabel, f),
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
            <Field label="Destinatário" error={errors.recipient}>
              {(c) => <TextInput {...c} autoComplete="name" maxLength={120} disabled={disabled} value={address.recipient} onChange={(e) => { setAddress({ recipient: e.target.value }); }} />}
            </Field>
            <Grid columns={2}>
              <Field label="CEP" error={errors.postal_code}>
                {(c) => (
                  <TextInput
                    {...c}
                    autoComplete="postal-code"
                    inputMode="numeric"
                    placeholder="00000-000"
                    disabled={disabled}
                    value={address.postal_code}
                    onChange={(e) => { setAddress({ postal_code: maskPostalCode(e.target.value) }); }}
                  />
                )}
              </Field>
              <Field label="UF" error={errors.state}>
                {(c) => (
                  <Select {...c} disabled={disabled} value={address.state} onChange={(e) => { setAddress({ state: e.target.value }); }}>
                    {states.map((s) => (
                      <option key={s} value={s}>
                        {s}
                      </option>
                    ))}
                  </Select>
                )}
              </Field>
            </Grid>
            <Field label="Logradouro" error={errors.street}>
              {(c) => <TextInput {...c} autoComplete="address-line1" maxLength={120} disabled={disabled} value={address.street} onChange={(e) => { setAddress({ street: e.target.value }); }} />}
            </Field>
            <Grid columns={2}>
              <Field label="Número" error={errors.number}>
                {(c) => <TextInput {...c} maxLength={120} disabled={disabled} value={address.number} onChange={(e) => { setAddress({ number: e.target.value }); }} />}
              </Field>
              <Field label="Cidade" error={errors.city}>
                {(c) => <TextInput {...c} autoComplete="address-level2" maxLength={120} disabled={disabled} value={address.city} onChange={(e) => { setAddress({ city: e.target.value }); }} />}
              </Field>
            </Grid>
          </Stack>
        </Stack>
      </Panel.Body>
    </Panel>
  );
}
