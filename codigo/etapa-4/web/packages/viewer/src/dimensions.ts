const mm = new Intl.NumberFormat("pt-BR", { maximumFractionDigits: 1 });

/** dimensions writes a bounding box as "10 × 20 × 30 mm". */
export function dimensions(bbox: readonly [number, number, number]): string {
  return `${mm.format(bbox[0])} × ${mm.format(bbox[1])} × ${mm.format(bbox[2])} mm`;
}
