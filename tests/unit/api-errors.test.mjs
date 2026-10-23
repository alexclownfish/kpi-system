import assert from 'node:assert/strict'
import fs from 'node:fs/promises'
import ts from 'typescript'
import axios from 'axios'

const source = await fs.readFile(new URL('../../lib/api.ts', import.meta.url),'utf8')
const tree = ts.createSourceFile('api.ts',source,ts.ScriptTarget.Latest,true)
const method = tree.statements.find(node=>ts.isFunctionDeclaration(node)&&node.name?.text==='getApiErrorMessage')
const js = ts.transpileModule(method.getText(tree).replace('export ',''),{compilerOptions:{target:ts.ScriptTarget.ES2022}}).outputText
const message = Function('axios', `${js};return getApiErrorMessage`)(axios)
for(const [payload,expected] of [
  [{error:{code:'TEMPLATE_SCOPE_MISMATCH',message:'岗位不匹配'}},'岗位不匹配'],
  [{error:'无法匹配考核模板',message:'未找到软件应用开发工程师适用模板'},'未找到软件应用开发工程师适用模板'],
  [{error:'缺少评分职责'},'缺少评分职责'],
  [{message:'详细原因'},'详细原因'],
]) {
  const error = new axios.AxiosError('request failed');error.response={data:payload}
  assert.equal(message(error),expected)
}
const page = await fs.readFile(new URL('../../app/evaluations/page.tsx',import.meta.url),'utf8')
assert.match(page,/getApiErrorMessage/, 'assessment page must not render error objects')
const pageTree = ts.createSourceFile('page.tsx',page,ts.ScriptTarget.Latest,true,ts.ScriptKind.TSX)
let subtreeBranch
function findSubtreeBranch(node) {
  if (ts.isIfStatement(node) && node.expression.getText(pageTree) === 'currentUser?.data_scope === "DEPARTMENT_TREE"') subtreeBranch = node.thenStatement
  ts.forEachChild(node,findSubtreeBranch)
}
findSubtreeBranch(pageTree)
assert.ok(subtreeBranch, 'team view must respect subtree scope')
const subtreeJS = ts.transpileModule(subtreeBranch.getText(pageTree),{}).outputText
const filterTeam = Function('employeeFilter', `const params={};${subtreeJS};return params`)
assert.deepEqual(filterTeam('all'), {}, 'no implicit department or manager narrowing')
assert.deepEqual(filterTeam('department:8'), {department_id:'8'})
assert.deepEqual(filterTeam('15'), {employee_id:'15'})
console.log('PASS: structured and legacy errors render detailed text')
