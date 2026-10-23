import assert from 'node:assert/strict'
import fs from 'node:fs/promises'
import ts from 'typescript'

const source = await fs.readFile(new URL('../../lib/api.ts', import.meta.url), 'utf8')
const tree = ts.createSourceFile('api.ts', source, ts.ScriptTarget.Latest, true)
let method
function visit(node) {
  if (ts.isVariableDeclaration(node) && node.name.getText(tree) === 'departmentApi') {
    method = node.initializer.properties.find(property => property.name.getText(tree) === 'getCatalog').initializer
  }
  ts.forEachChild(node, visit)
}
visit(tree)
assert.ok(method, 'department catalog method exists')
const js = ts.transpileModule(`(${method.getText(tree)})`, { compilerOptions: { target: ts.ScriptTarget.ES2022 } }).outputText
const calls = []
const stub = { getAll: async params => {
  calls.push(params)
  const start = (params.page - 1) * 100
  return { data: Array.from({length: Math.min(100, 205-start)}, (_,index) => ({id:start+index+1})), totalPages:3 }
} }
const getCatalog = Function('departmentApi', `return ${js}`)(stub)
const data = await getCatalog()
assert.equal(data.length,205)
assert.deepEqual(calls,[{page:1,pageSize:100},{page:2,pageSize:100},{page:3,pageSize:100}])
assert.equal(data[204].id,205)
stub.getAll = async () => { throw new Error('Read failed') }
await assert.rejects(getCatalog(), /Read failed/)
console.log('PASS: all 205 departments loaded, valid page sizes, failures propagated')
