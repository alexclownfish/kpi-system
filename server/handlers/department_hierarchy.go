package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"dootask-kpi-server/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func seedHongxingHierarchy(db *gorm.DB) error {
	const key = "hongxing-hierarchy-v1"
	return db.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&models.BusinessInitialization{}).Where("key = ?", key).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return nil
		}
		find := func(name string) (models.Department, error) {
			var found []models.Department
			if err := tx.Where("name = ?", name).Find(&found).Error; err != nil {
				return models.Department{}, err
			}
			if len(found) > 1 {
				return models.Department{}, fmt.Errorf("部门“%s”重名，无法初始化层级", name)
			}
			if len(found) == 1 {
				return found[0], nil
			}
			d := models.Department{Name: name}
			err := tx.Create(&d).Error
			return d, err
		}
		parent, err := find("机器人事业研发部")
		if err != nil {
			return err
		}
		for _, name := range []string{"运控部", "结构部门", "嵌入式开发部", "软件应用开发部"} {
			child, err := find(name)
			if err != nil {
				return err
			}
			if child.ParentID != nil && *child.ParentID != parent.ID {
				return fmt.Errorf("部门“%s”已有其他上级，请确认后再初始化", name)
			}
			if err := validateDepartmentParent(tx, child.ID, &parent.ID); err != nil {
				return err
			}
			if err := tx.Model(&child).Update("parent_id", parent.ID).Error; err != nil {
				return err
			}
		}
		if err := tx.Create(&models.BusinessInitialization{Key: key, SourceSHA256: "user-confirmed-20261001"}).Error; err != nil {
			return err
		}
		return tx.Create(&models.AuditLog{Action: "initialize_department_hierarchy", Resource: "business_catalog", ResourceID: key, Result: "SUCCESS", Details: "机器人事业研发部：运控、结构、嵌入式、软件应用开发"}).Error
	})
}

// Repair only missing direct-manager links for the known Hongxing subdepartment
// heads. Existing HR assignments are authoritative and are never overwritten.
func repairDepartmentLeaderManagers(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var parent models.Department
		if err := tx.Where("name = ?", "机器人事业研发部").First(&parent).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		for _, name := range []string{"运控部", "结构部门", "嵌入式开发部", "软件应用开发部"} {
			var child models.Department
			if err := tx.Where("name = ?", name).First(&child).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					continue
				}
				return err
			}
			if child.ParentID == nil || *child.ParentID != parent.ID || child.LeaderID == nil {
				continue
			}
			var head models.Employee
			if err := tx.First(&head, *child.LeaderID).Error; err != nil || !head.IsActive || head.ManagerID != nil || head.DepartmentID != child.ID {
				continue
			}
			supervisors, source, err := resolveHeadSupervisor(tx, head)
			if err != nil || len(supervisors) != 1 || supervisors[0].ID == head.ID {
				continue
			}
			if err := tx.Model(&head).Update("manager_id", supervisors[0].ID).Error; err != nil {
				return err
			}
			if err := tx.Create(&models.AuditLog{
				Action:     "repair_department_leader_manager",
				Resource:   "employee",
				ResourceID: strconv.FormatUint(uint64(head.ID), 10),
				Result:     "SUCCESS",
				Details:    "department=" + name + ";manager_id=" + strconv.FormatUint(uint64(supervisors[0].ID), 10) + ";source=" + source,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func validateDepartmentParent(db *gorm.DB, id uint, parentID *uint) error {
	if parentID == nil {
		return nil
	}
	if *parentID == 0 || *parentID == id {
		return errors.New("上级部门不能为自身或无效部门")
	}
	ancestors, err := models.DepartmentAncestors(db, *parentID)
	if err != nil {
		return fmt.Errorf("上级部门无效：%w", err)
	}
	for _, ancestor := range ancestors {
		if ancestor == id {
			return errors.New("上级部门不能为下属部门，部门层级不能循环")
		}
	}
	return nil
}

func canonicalEmployeePosition(db *gorm.DB, employee models.Employee) string {
	position := strings.TrimSpace(employee.Position)
	if position == "工程师" || position == "部门负责人" {
		return position
	}
	var department models.Department
	if db.Select("name").First(&department, employee.DepartmentID).Error != nil {
		return position
	}
	// The source workbook models the product department with only two subject
	// templates. In that department, "产品经理" is the department-head subject
	// role used by 09-产品部门-部门负责人.
	if department.Name == "产品部门" && (position == "产品经理" || position == "产品负责人") {
		return "部门负责人"
	}
	var catalog hongxingCatalog
	if json.Unmarshal(hongxingCatalogJSON, &catalog) != nil {
		return position
	}
	known := false
	for _, profile := range catalog.Profiles {
		if profile.Department == department.Name {
			known = true
			break
		}
	}
	if !known {
		return position
	}
	stem := strings.TrimSuffix(strings.TrimSuffix(department.Name, "部门"), "部")
	for _, prefix := range []string{department.Name, stem} {
		if !strings.HasPrefix(position, prefix) {
			continue
		}
		switch strings.TrimPrefix(position, prefix) {
		case "工程师":
			return "工程师"
		case "负责人", "部门负责人":
			return "部门负责人"
		}
	}
	return position
}

func activeResponsibilities(db *gorm.DB, code string) *gorm.DB {
	now := time.Now()
	return db.Model(&models.EvaluationResponsibility{}).Preload("User").Where("UPPER(code) = ? AND active = ? AND project_id IS NULL", code, true).
		Where("(effective_from IS NULL OR effective_from <= ?) AND (effective_to IS NULL OR effective_to >= ?)", now, now)
}

func responsibilityUsers(query *gorm.DB, excluded uint) ([]models.Employee, error) {
	var rows []models.EvaluationResponsibility
	if err := query.Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	users := []models.Employee{}
	seen := map[uint]bool{}
	for _, row := range rows {
		if row.User.ID == 0 || row.User.ID == excluded || !row.User.IsActive || seen[row.User.ID] {
			continue
		}
		seen[row.User.ID] = true
		users = append(users, row.User)
	}
	return users, nil
}

func isDepartmentHead(db *gorm.DB, employee models.Employee) bool {
	var department models.Department
	if db.Select("leader_id").First(&department, employee.DepartmentID).Error == nil && department.LeaderID != nil && *department.LeaderID == employee.ID && employee.ID != 0 {
		return true
	}
	return canonicalEmployeePosition(db, employee) == "部门负责人"
}

// An explicit organizational owner supersedes duties, but invalid owners never silently fall back.
func explicitDepartmentLeader(db *gorm.DB, departmentID, excluded uint) ([]models.Employee, bool, error) {
	var department models.Department
	if err := db.First(&department, departmentID).Error; err != nil {
		return nil, false, err
	}
	if department.LeaderID == nil {
		return nil, false, nil
	}
	var leader models.Employee
	if err := db.First(&leader, *department.LeaderID).Error; err != nil || !leader.IsActive {
		return nil, true, fmt.Errorf("部门“%s”的负责人已失效，请重新设置在职负责人", department.Name)
	}
	if leader.ID == excluded {
		return nil, true, errors.New("部门负责人评分人不能为被考核者本人")
	}
	return []models.Employee{leader}, true, nil
}

func managerAssignmentError(db *gorm.DB, employee models.Employee, managerID uint) error {
	if managerID == 0 || managerID == employee.ID {
		return errors.New("直属上级不能为自己或无效员工")
	}
	var manager models.Employee
	if err := db.First(&manager, managerID).Error; err != nil {
		return errors.New("直属上级不存在")
	}
	if !manager.IsActive {
		return errors.New("直属上级已停用")
	}
	seen := map[uint]bool{}
	current := manager
	for {
		if current.ID == employee.ID && employee.ID != 0 || seen[current.ID] {
			return errors.New("直属上级关系不能循环")
		}
		seen[current.ID] = true
		if current.ManagerID == nil {
			break
		}
		id := *current.ManagerID
		current = models.Employee{}
		if err := db.First(&current, id).Error; err != nil {
			return errors.New("上级链存在无效员工")
		}
	}
	var department models.Department
	if err := db.First(&department, employee.DepartmentID).Error; err != nil {
		return errors.New("所选部门不存在")
	}
	if department.LeaderID != nil && *department.LeaderID == managerID {
		return nil
	}
	if !isDepartmentHead(db, employee) {
		if manager.DepartmentID != employee.DepartmentID {
			return errors.New("直属上级不属于所选部门，且未被任命为该部门负责人")
		}
		return nil
	}

	ctos, err := responsibilityUsers(activeResponsibilities(db, "CTO").Where("scope_type = 'company'"), employee.ID)
	if err != nil {
		return err
	}
	for _, cto := range ctos {
		if cto.ID == managerID {
			return nil
		}
	}
	ancestors, err := models.DepartmentAncestors(db, employee.DepartmentID)
	if err != nil {
		return err
	}
	for _, id := range ancestors {
		leaders, configured, err := explicitDepartmentLeader(db, id, employee.ID)
		if err != nil {
			return err
		}
		if configured {
			if leaders[0].ID == managerID {
				return nil
			}
			continue
		}
		users, err := responsibilityUsers(activeResponsibilities(db, "DEPARTMENT_MANAGER").Where("scope_type = 'department' AND department_id = ?", id), employee.ID)
		if err != nil {
			return err
		}
		for _, user := range users {
			if user.ID == managerID && user.DepartmentID == id {
				return nil
			}
		}
	}
	return errors.New("部门负责人直属上级必须是上级部门负责人或公司 CTO，请先设置上级部门及其负责人，或维护 CTO 评分职责")
}

func resolveHeadSupervisor(db *gorm.DB, employee models.Employee) ([]models.Employee, string, error) {
	if employee.ManagerID != nil {
		if err := managerAssignmentError(db, employee, *employee.ManagerID); err != nil {
			return nil, "DIRECT_MANAGER", err
		}
		var manager models.Employee
		if err := db.First(&manager, *employee.ManagerID).Error; err != nil {
			return nil, "DIRECT_MANAGER", err
		}
		return []models.Employee{manager}, "DIRECT_MANAGER", nil
	}
	ancestors, err := models.DepartmentAncestors(db, employee.DepartmentID)
	if err != nil {
		return nil, "PARENT_DEPARTMENT_MANAGER", err
	}
	for _, id := range ancestors {
		leaders, configured, err := explicitDepartmentLeader(db, id, employee.ID)
		if err != nil {
			return nil, "PARENT_DEPARTMENT_LEADER", err
		}
		if configured {
			if err := managerAssignmentError(db, employee, leaders[0].ID); err != nil {
				return nil, "PARENT_DEPARTMENT_LEADER", err
			}
			return leaders, "PARENT_DEPARTMENT_LEADER", nil
		}
		users, err := responsibilityUsers(activeResponsibilities(db, "DEPARTMENT_MANAGER").Where("scope_type = 'department' AND department_id = ?", id), employee.ID)
		if err != nil {
			return nil, "PARENT_DEPARTMENT_MANAGER", err
		}
		valid := []models.Employee{}
		for _, user := range users {
			if user.DepartmentID == id {
				valid = append(valid, user)
			}
		}
		if len(valid) > 1 {
			return nil, "PARENT_DEPARTMENT_MANAGER", errors.New("上级部门存在多位负责人，请明确选择直属上级")
		}
		if len(valid) == 1 {
			if err := managerAssignmentError(db, employee, valid[0].ID); err != nil {
				return nil, "PARENT_DEPARTMENT_MANAGER", err
			}
			return valid, "PARENT_DEPARTMENT_MANAGER", nil
		}
	}
	users, err := responsibilityUsers(activeResponsibilities(db, "CTO").Where("scope_type = 'company'"), employee.ID)
	if err != nil {
		return nil, "COMPANY_CTO", err
	}
	if len(users) > 1 {
		return nil, "COMPANY_CTO", errors.New("公司有多位 CTO，请明确选择直属上级")
	}
	if len(users) == 0 {
		return nil, "COMPANY_CTO", errors.New("缺少上级部门负责人或公司 CTO 评分职责")
	}
	if err := managerAssignmentError(db, employee, users[0].ID); err != nil {
		return nil, "COMPANY_CTO", err
	}
	return users, "COMPANY_CTO", nil
}

func GetDepartmentManagerCandidates(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		c.JSON(400, gin.H{"error": "无效部门ID"})
		return
	}
	var department models.Department
	if err := models.DB.First(&department, id).Error; err != nil {
		c.JSON(404, gin.H{"error": "部门不存在"})
		return
	}
	employeeID := uint64(0)
	if raw := c.Query("employee_id"); raw != "" {
		employeeID, err = strconv.ParseUint(raw, 10, 32)
		if err != nil || employeeID == 0 {
			c.JSON(400, gin.H{"error": "无效员工ID"})
			return
		}
		if !CanAccessEmployee(actorID(c), uint(employeeID)) {
			c.JSON(403, gin.H{"error": "员工超出数据范围"})
			return
		}
	}
	employee := models.Employee{ID: uint(employeeID), DepartmentID: uint(id), Position: c.Query("position")}
	var visible []models.Employee
	if err := ApplyEmployeeScope(models.DB.Model(&models.Employee{}), actorID(c)).Where("is_active = ?", true).Preload("Department").Find(&visible).Error; err != nil {
		c.JSON(500, gin.H{"error": "上级候选加载失败"})
		return
	}
	// Reveal only the appointed leader as a candidate to authorized operators
	// working on this department; do not expose all of the leader's department.
	if department.LeaderID != nil {
		var actor models.Employee
		_ = models.DB.Select("department_id").First(&actor, actorID(c)).Error
		if actor.DepartmentID == department.ID || departmentWithinActorScope(actorID(c), department.ID) {
			var appointed models.Employee
			if models.DB.First(&appointed, *department.LeaderID).Error == nil {
				visible = append(visible, appointed)
			}
		}
	}
	candidates := []models.Employee{}
	seen := map[uint]bool{}
	for _, user := range visible {
		if !seen[user.ID] && managerAssignmentError(models.DB, employee, user.ID) == nil {
			seen[user.ID] = true
			candidates = append(candidates, user)
		}
	}
	response := gin.H{"data": candidates}
	if len(candidates) == 0 && isDepartmentHead(models.DB, employee) {
		if _, _, err := resolveHeadSupervisor(models.DB, employee); err != nil {
			response["message"] = err.Error()
		}
	}
	c.JSON(http.StatusOK, response)
}
