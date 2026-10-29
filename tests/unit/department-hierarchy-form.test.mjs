import assert from 'node:assert/strict'
import fs from 'node:fs/promises'
import ts from 'typescript'

const source = await fs.readFile(new URL('../../app/departments/page.tsx',import.meta.url),'utf8')
const tree = ts.createSourceFile('page.tsx',source,ts.ScriptTarget.Latest,true,ts.ScriptKind.TSX)
const method = tree.statements.find(node=>ts.isFunctionDeclaration(node)&&node.name?.text==='getParentDepartmentOptions')
assert.ok(method, 'parent options must exclude self and descendants')
const js=ts.transpileModule(method.getText(tree),{}).outputText
const getOptions=Function(`${js};return getParentDepartmentOptions`)()
const departments=[{id:1},{id:2,parent_id:1},{id:3,parent_id:2},{id:4}]
assert.deepEqual(getOptions(departments,1).map(d=>d.id),[4])
assert.deepEqual(getOptions(departments,2).map(d=>d.id),[1,4])
assert.equal(getOptions(departments).length,4)
assert.deepEqual(getOptions([{id:1,parent_id:2},{id:2,parent_id:1},{id:3}],1).map(d=>d.id),[3])
assert.match(source, /id="department-leader"/)
assert.match(source, /<TableHead>上级部门<\/TableHead>/)
assert.match(source, /<TableHead>部门负责人<\/TableHead>/)
assert.match(source, /departmentApi\.getLeaderCandidates\(\{ page: 1, pageSize: 100 \}\)/)
assert.match(source, /page <= first\.totalPages/)
assert.match(source, /department\.leader\.name/)
assert.match(source, /previous_leader_direct_reports/)
assert.match(source, /未自动改写/)
assert.match(source, /if \(!dialogOpen \|\| !canChooseLeader\) return/)
assert.doesNotMatch(source, /editingDepartment && <div[\s\S]*?id="department-leader"/)
console.log('PASS: parent options and explicit department leader form')
