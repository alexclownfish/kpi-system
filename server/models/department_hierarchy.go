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
