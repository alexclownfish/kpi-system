import assert from "node:assert/strict"
import fs from "node:fs"
import ts from "typescript"

const source = fs.readFileSync(new URL("../../lib/template-weights.ts", import.meta.url), "utf8")
const js = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2020, module: ts.ModuleKind.ES2020 } }).outputText
const { compositeWeight, compositeWeightTotal, validateCalculationWeights, templateScoreMaximum } = await import(`data:text/javascript;base64,${Buffer.from(js).toString("base64")}`)
const manifest = JSON.parse(fs.readFileSync(new URL("../../specs/006-performance-calculation-engine/source-workbook-manifest-20261001.json", import.meta.url)))
for (const profile of manifest.cases) {
  assert.equal(compositeWeightTotal(profile.dimensions), 100, profile.sheet + "/" + profile.position)
  assert.deepEqual(validateCalculationWeights(profile.dimensions), [])
}
assert.equal(compositeWeight(16.2, 25), 4.1)
assert.equal(compositeWeight(15, 73), 11)
assert.equal(compositeWeight(10, 0.5), 0.1)
assert.equal(compositeWeight(1e-7, 100), 0)
assert.equal(validateCalculationWeights([{ weight: 100, items: [{ item_weight: 99 }] }]).length, 1)
assert.equal(validateCalculationWeights([{ weight: 100, items: [{ item_weight: 33.33 }, { item_weight: 33.33 }, { item_weight: 33.34 }] }]).length, 1)
assert.deepEqual(validateCalculationWeights([{ weight: 40, items: [{ item_weight: 50 }] }, { weight: 80, items: [{ item_weight: 100 }] }]), [])
for (const invalid of [NaN, Infinity, -1, 101]) {
  assert.ok(validateCalculationWeights([{ weight: invalid, items: [{ item_weight: 100 }] }]).length)
  assert.ok(validateCalculationWeights([{ weight: 100, items: [{ item_weight: invalid }] }]).length)
}
assert.ok(validateCalculationWeights([]).length)
assert.ok(validateCalculationWeights([{ weight: 100, items: [] }]).length)
assert.equal(templateScoreMaximum({ dimensions: [{}] }, [{ max_score: 100 }, { max_score: 100 }]), 100)
assert.equal(templateScoreMaximum({}, [{ max_score: 100 }, { max_score: 50 }]), 150)
console.log("PASS: 18 source profiles, decimal rounding, range and composite-total frontend checks")
