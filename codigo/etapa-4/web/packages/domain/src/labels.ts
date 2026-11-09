// The catalog keys in Portuguese. An unknown key shows as it is.
const quality: Record<string, string> = { draft: "Rascunho", standard: "Padrão", fine: "Fina" };
const finishing: Record<string, string> = { none: "Sem acabamento", sanded: "Lixado", painted: "Pintado" };
const color: Record<string, string> = { black: "Preto", white: "Branco", gray: "Cinza", red: "Vermelho", blue: "Azul", clear: "Transparente" };
const material: Record<string, string> = { pla: "PLA", petg: "PETG", nylon: "Nylon PA12" };

const pick = (names: Record<string, string>) => (key: string) => names[key] ?? key;

export const qualityLabel = pick(quality);
export const finishingLabel = pick(finishing);
export const colorLabel = pick(color);
export const materialLabel = pick(material);
