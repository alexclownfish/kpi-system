package models

import (
	"fmt"

	"gorm.io/gorm"
)

// UNION (not UNION ALL) also terminates on corrupt pre-existing cycles.
const DepartmentSubtreeSQL = `WITH RECURSIVE department_tree(id) AS (
 SELECT id FROM departments WHERE id = ?
 UNION SELECT d.id FROM departments d JOIN department_tree t ON d.parent_id = t.id
) SELECT id FROM department_tree`

const UserDepartmentSubtreeSQL = `WITH RECURSIVE department_tree(id) AS (
 SELECT department_id FROM employees WHERE id = ?
 UNION SELECT d.id FROM departments d JOIN department_tree t ON d.parent_id = t.id
) SELECT id FROM department_tree`

// UserManagedDepartmentSQL returns departments explicitly led by the user,
// plus the user's own department. Department-tree scopes then include their
// descendants. This keeps organizational assignment separate from RBAC.
const UserManagedDepartmentSQL = `WITH managed_roots(id) AS (
 SELECT department_id FROM employees WHERE id = ? AND is_active = TRUE
 UNION
 SELECT id FROM departments WHERE leader_id = ?
 AND EXISTS (SELECT 1 FROM employees leader WHERE leader.id = departments.leader_id AND leader.is_active = TRUE)
) SELECT id FROM managed_roots`

const UserManagedDepartmentTreeSQL = `WITH RECURSIVE managed_roots(id) AS (
 SELECT department_id FROM employees WHERE id = ? AND is_active = TRUE
 UNION
 SELECT id FROM departments WHERE leader_id = ?
 AND EXISTS (SELECT 1 FROM employees leader WHERE leader.id = departments.leader_id AND leader.is_active = TRUE)
), managed_departments(id) AS (
 SELECT id FROM managed_roots
 UNION
 SELECT d.id FROM departments d JOIN managed_departments m ON d.parent_id = m.id
) SELECT id FROM managed_departments`

func DepartmentAncestors(db *gorm.DB, departmentID uint) ([]uint, error) {
	ids := []uint{}
	seen := map[uint]bool{}
	for departmentID != 0 {
		if seen[departmentID] {
			return nil, fmt.Errorf("部门层级存在循环")
		}
		seen[departmentID] = true
		var department Department
		if err := db.Select("id", "parent_id").First(&department, departmentID).Error; err != nil {
			return nil, err
		}
		if department.ParentID == nil {
			break
		}
		departmentID = *department.ParentID
		ids = append(ids, departmentID)
	}
	return ids, nil
}
