import assert from 'node:assert/strict'
import fs from 'node:fs/promises'

const roles = await fs.readFile(new URL('../../app/roles/page.tsx', import.meta.url), 'utf8')
assert.match(roles, /canEdit && role.code !== "super_admin"/)
assert.match(roles, /disabled=\{editingRole\?\.is_system\}/)
assert.match(roles, /canDelete && !role.is_system/)
assert.match(roles, /getApiErrorMessage as errorMessage/)
const evaluations = await fs.readFile(new URL('../../app/evaluations/page.tsx', import.meta.url), 'utf8')
assert.match(evaluations, /templatePreview.ready === false && hasPermission\("responsibility:view"\)/)
assert.match(evaluations, /href="\/evaluation-responsibilities"/)
assert.match(evaluations, /templatePreview.workflow_mode === "enabled"/)
console.log('PASS: system role editor protection and creation readiness UI contract')
