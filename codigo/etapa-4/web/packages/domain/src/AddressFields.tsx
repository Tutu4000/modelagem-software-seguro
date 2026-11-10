import { Field, Grid, Select, Stack, TextInput } from "@nugget/ui";

import { maskPostalCode, states, type Address, type AddressErrors } from "./address";

export interface AddressFieldsProps {
  address: Address;
  onChange: (a: Address) => void;
  /** Shown once the person tried to send. */
  errors: AddressErrors;
  disabled?: boolean;
}

/** AddressFields is the delivery address form of the customer's quote and of
 * the support's address change, with the same mask and the same rules. */
export function AddressFields({ address, onChange, errors, disabled = false }: AddressFieldsProps) {
  const setAddress = (patch: Partial<Address>) => { onChange({ ...address, ...patch }); };
  return (
    <Stack gap={3}>
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
  );
}
