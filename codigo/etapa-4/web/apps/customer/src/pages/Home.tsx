import { Button, CheckIcon, CubeIcon, Grid, Panel, ShieldIcon, Stack, TruckIcon, UploadIcon } from "@nugget/ui";

import styles from "./Home.module.css";

const steps = [
  { title: "Envie o STL", text: "O arquivo é validado e processado num ambiente isolado." },
  { title: "Configure e orce", text: "Material, cor, qualidade e quantidade. O preço sai do servidor, na hora." },
  { title: "Revisão técnica", text: "Um técnico confere a manufaturabilidade e registra o laudo." },
  { title: "Produção e entrega", text: "Pague, acompanhe a impressão e rastreie o envio." },
];

const materials = [
  { name: "PLA", text: "Peças de uso geral e protótipos.", colors: "preto, branco, cinza" },
  { name: "PETG", text: "Mais resistente ao calor e a impactos.", colors: "preto, branco, transparente" },
  { name: "ABS", text: "Peças técnicas e de encaixe.", colors: "preto, cinza" },
];

const guarantees = [
  { icon: <ShieldIcon />, text: "Seu projeto só é aberto pelo técnico atribuído ao pedido." },
  { icon: <CheckIcon />, text: "O pagamento só vale com a confirmação assinada do gateway." },
  { icon: <TruckIcon />, text: "A troca de endereço só acontece com o código enviado ao seu e-mail." },
];

export function Home() {
  return (
    <Stack gap={4}>
      <section className={styles.hero} aria-labelledby="hero-title">
        <Stack gap={4}>
          <h1 id="hero-title">Envie seu STL e receba o orçamento</h1>
          <p className={styles.lead}>Impressão 3D sob demanda, com revisão técnica antes da produção e acompanhamento até a entrega.</p>
        </Stack>
        <Panel accent>
          <Panel.Header>
            <h2>Novo orçamento</h2>
            <CubeIcon />
          </Panel.Header>
          <Panel.Body>
            <div className={styles.drop}>
              <UploadIcon />
              <span>Arraste o arquivo STL aqui</span>
              <small>Até 100 MB. Disponível depois do login.</small>
            </div>
          </Panel.Body>
          <Panel.Footer>
            <Button variant="primary" block disabled>
              Entrar para orçar
            </Button>
          </Panel.Footer>
        </Panel>
      </section>

      <section className={styles.section} aria-labelledby="how-title">
        <Stack gap={4}>
          <h2 id="how-title">Como funciona</h2>
          <Grid columns={4}>
            {steps.map((s, i) => (
              <Panel key={s.title}>
                <Panel.Body>
                  <Stack gap={2}>
                    <span className={styles.stepNumber}>{String(i + 1).padStart(2, "0")}</span>
                    <h3>{s.title}</h3>
                    <p>{s.text}</p>
                  </Stack>
                </Panel.Body>
              </Panel>
            ))}
          </Grid>
        </Stack>
      </section>

      <section className={styles.section} aria-labelledby="materials-title">
        <Stack gap={4}>
          <h2 id="materials-title">Materiais</h2>
          <Grid columns={3}>
            {materials.map((m) => (
              <Panel key={m.name}>
                <Panel.Header>
                  <h3>{m.name}</h3>
                </Panel.Header>
                <Panel.Body>
                  <Stack gap={2}>
                    <p>{m.text}</p>
                    <small className={styles.materialPrice}>Cores: {m.colors}</small>
                  </Stack>
                </Panel.Body>
              </Panel>
            ))}
          </Grid>
        </Stack>
      </section>

      <section className={styles.section} aria-labelledby="trust-title">
        <Stack gap={4}>
          <h2 id="trust-title">Seu projeto protegido</h2>
          <Grid columns={3}>
            {guarantees.map((g) => (
              <Stack key={g.text} direction="row" gap={3} align="start">
                {g.icon}
                <p>{g.text}</p>
              </Stack>
            ))}
          </Grid>
        </Stack>
      </section>
    </Stack>
  );
}
