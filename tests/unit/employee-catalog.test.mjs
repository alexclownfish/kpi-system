import assert from 'node:assert/strict'
import fs from 'node:fs/promises'
import test from 'node:test'
import ts from 'typescript'

const apiSource = await fs.readFile(new URL('../../lib/api.ts', import.meta.url), 'utf8')
const pageSource = await fs.readFile(new URL('../../app/evaluation-responsibilities/page.tsx', import.meta.url), 'utf8')
function findVariable(source, name, filename = 'api.ts') {
  const tree = ts.createSourceFile(filename, source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX)
  let result
  function visit(node) {
    if (ts.isVariableDeclaration(node) && node.name.getText(tree) === name) result = node.initializer
    ts.forEachChild(node, visit)
  }
  visit(tree)
  assert.ok(result, `${name} exists`)
  return { tree, result }
}
function compile(node, tree, dependencies) {
  const js = ts.transpileModule(`(${node.getText(tree)})`, { compilerOptions: { target: ts.ScriptTarget.ES2022 } }).outputText
  return Function(...Object.keys(dependencies), `return ${js}`)(...Object.values(dependencies))
}
function catalog(stub) {
  const { tree, result } = findVariable(apiSource, 'employeeApi')
  const method = result.properties.find(property => property.name.getText(tree) === 'getCatalog')
  assert.ok(method, 'employee catalog must exist: the old 500-size request silently returns only ten employees')
  return compile(method.initializer, tree, { employeeApi: stub })
}

for (const total of [0, 17, 100, 101, 205]) {
  test(`complete employee catalog with ${total} employees`, async () => {
    const calls = []
    const people = Array.from({ length: total }, (_, index) => ({ id: index + 1, role: index === 10 ? 'cto' : index === 11 ? 'manager' : 'employee' }))
    const filters = { is_active: true, department_id: '8', search: '工程师' }
    const getCatalog = catalog({ getAll: async params => {
      calls.push(params)
      return { data: people.slice((params.page - 1) * 100, params.page * 100), totalPages: Math.ceil(total / 100) }
    } })
    assert.deepEqual(await getCatalog(filters), people)
    assert.deepEqual(calls, Array.from({ length: Math.max(1, Math.ceil(total / 100)) }, (_, index) => ({ ...filters, page: index + 1, pageSize: 100 })))
    assert.equal(calls.some(params => 'role' in params), false)
  })
}

test('empty nullable data and default filters', async () => {
  const calls = []
  const getCatalog = catalog({ getAll: async params => { calls.push(params); return { data: null, totalPages: 0 } } })
  assert.deepEqual(await getCatalog(), [])
  assert.deepEqual(calls, [{ page: 1, pageSize: 100 }])
})

test('second page failure rejects the entire catalog', async () => {
  let calls = 0
  const getCatalog = catalog({ getAll: async () => {
    if (++calls === 2) throw new Error('Second page failed')
    return { data: [{ id: 1 }], totalPages: 3 }
  } })
  await assert.rejects(getCatalog({ is_active: true }), /Second page failed/)
  assert.equal(calls, 2)
})

test('page uses unfiltered active catalog and blocks stale data after failure', async () => {
  assert.match(pageSource, /employeeApi\.getCatalog\(\{ is_active: true \}\)/)
  assert.doesNotMatch(pageSource, /pageSize: 500/)
  const { tree, result } = findVariable(pageSource, 'load', 'page.tsx')
  const state = { items: ['stale'], departments: ['stale'], employees: ['stale'], loading: false, loadError: null }
  const alerts = []
  const employeeApi = { getCatalog: async () => { throw new Error('Second page failed') } }
  const dependencies = {
    evaluationResponsibilityApi: { getAll: async () => ({ data: { items: [] } }) },
    departmentApi: { getCatalog: async () => [] }, employeeApi,
    Alert: async (...args) => alerts.push(args), errorMessage: error => error.message,
  }
  for (const key of Object.keys(state)) dependencies[`set${key[0].toUpperCase()}${key.slice(1)}`] = value => { state[key] = value }
  const load = compile(result.arguments[0], tree, dependencies)
  await load()
  assert.deepEqual(state, { items: [], departments: [], employees: [], loading: false, loadError: 'Second page failed' })
  assert.deepEqual(alerts, [['加载失败', 'Second page failed']])
  employeeApi.getCatalog = async () => [{ id: 11, role: 'cto' }, { id: 12, role: 'manager' }, { id: 15, role: 'employee' }]
  await load()
  assert.equal(state.loadError, null)
  assert.equal(state.employees.length, 3)
  assert.match(pageSource, /disabled=\{loading \|\| !!loadError\}/)
  assert.match(pageSource, /disabled=\{loading \|\| !!loadError \|\| employees\.length === 0\}/)
  assert.match(pageSource, /disabled=\{saving \|\| loading \|\| !!loadError \|\| employees\.length === 0\}/)
  assert.match(pageSource, /暂无可选启用员工/)
})

test('save guard prevents loading, failed, and empty catalog mutations', async () => {
  const { tree, result } = findVariable(pageSource, 'save', 'page.tsx')
  for (const state of [
    { loading: true, loadError: null, employees: [{ id: 11 }] },
    { loading: false, loadError: 'failed', employees: [{ id: 11 }] },
    { loading: false, loadError: null, employees: [] },
  ]) {
    const save = compile(result, tree, state)
    // Any access to form or mutation APIs would throw, proving early return.
    await save()
  }
})
