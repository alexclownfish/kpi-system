type WeightDimension = { weight: number; items: { item_weight: number }[] }

export function templateScoreMaximum(template: { dimensions?: unknown[] }, items: { max_score: number }[]): number {
  return template.dimensions?.length ? 100 : items.reduce((total, item) => total + item.max_score, 0)
}

const validWeight = (value: number) => Number.isFinite(value) && value >= 0 && value <= 100

function decimalFraction(value: number): [bigint, bigint] {
  const [mantissa, exponent = "0"] = value.toString().split("e")
  const scale = (mantissa.split(".")[1]?.length || 0) - Number(exponent)
  const coefficient = BigInt(mantissa.replace(".", ""))
  const power = BigInt(10) ** BigInt(Math.abs(scale))
  return scale >= 0 ? [coefficient, power] : [coefficient * power, BigInt(1)]
}

// Match workbook ROUND(..., 1) without binary floating-point tie errors.
export function compositeWeight(dimension: number, item: number): number {
  if (!validWeight(dimension) || !validWeight(item)) return NaN
  const [dimensionNumerator, dimensionDenominator] = decimalFraction(dimension)
  const [itemNumerator, itemDenominator] = decimalFraction(item)
  const numerator = dimensionNumerator * itemNumerator
  const denominator = dimensionDenominator * itemDenominator * BigInt(10)
  return Number((numerator * BigInt(2) + denominator) / (denominator * BigInt(2))) / 10
}

export function compositeWeightTotal(dimensions: WeightDimension[]): number {
  return dimensions.reduce((total, dimension) => total + dimension.items.reduce(
    (subtotal, item) => subtotal + Math.round(compositeWeight(dimension.weight, item.item_weight) * 10), 0), 0) / 10
}

export function validateCalculationWeights(dimensions: WeightDimension[]): string[] {
  const errors: string[] = []
  if (!dimensions.length) errors.push("至少需要一个考核维度")
  for (const [index, dimension] of dimensions.entries()) {
    if (!validWeight(dimension.weight)) errors.push(`第 ${index + 1} 个维度权重必须为 0 到 100 之间的有限数值`)
    if (!dimension.items.length) errors.push(`第 ${index + 1} 个维度至少需要一个指标`)
    if (dimension.items.some(item => !validWeight(item.item_weight))) errors.push(`第 ${index + 1} 个维度的指标权重必须为 0 到 100 之间的有限数值`)
  }
  const total = compositeWeightTotal(dimensions)
  if (Number.isFinite(total) && total !== 100) errors.push(`指标综合权重合计必须为 100%，当前为 ${total.toFixed(1)}%`)
  return errors
}
