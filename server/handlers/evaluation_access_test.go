package handlers

import (
	"fmt"
	"net/http"
	"testing"

	"dootask-kpi-server/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupEvaluationAccessDB(t *testing.T) (*gorm.DB, models.Employee, models.Employee, models.Employee, models.EvaluationScoringTask) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:evaluation-access-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Department{}, &models.Employee{}, &models.KPITemplate{}, &models.KPIEvaluation{}, &models.KPIItem{}, &models.EvaluationScoringTask{}); err != nil {
		t.Fatal(err)
	}
	models.DB = db
	department := models.Department{Name: "权限测试部"}
	db.Create(&department)
	owner := models.Employee{Name: "评分人", Email: "task-owner@test", Password: "hash", DepartmentID: department.ID, IsActive: true}
	target := models.Employee{Name: "被考核人", Email: "task-target@test", Password: "hash", DepartmentID: department.ID, IsActive: true}
	outsider := models.Employee{Name: "无任务用户", Email: "task-outsider@test", Password: "hash", DepartmentID: department.ID, IsActive: true}
	db.Create(&owner)
	db.Create(&target)
	db.Create(&outsider)
	template := models.KPITemplate{Name: "权限模板", Period: "monthly"}
	db.Create(&template)
	evaluation := models.KPIEvaluation{EmployeeID: target.ID, TemplateID: template.ID, Period: "2026-09", Status: "scoring"}
	db.Create(&evaluation)
	task := models.EvaluationScoringTask{EvaluationID: evaluation.ID, ItemID: 1, ScorerUserID: owner.ID, ScorerType: "PM", Status: "pending", Required: true, Version: 2}
	db.Create(&task)
	return db, owner, target, outsider, task
}

func TestLoadScoringTaskForWriteAllowsAssignedOwner(t *testing.T) {
	db, owner, _, _, task := setupEvaluationAccessDB(t)
	loaded, err := loadScoringTaskForWrite(db, owner.ID, task.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != task.ID {
		t.Fatalf("loaded task=%+v", loaded)
	}
}

func TestLoadScoringTaskForWriteDeniesUnassignedUser(t *testing.T) {
	db, _, _, outsider, task := setupEvaluationAccessDB(t)
	_, err := loadScoringTaskForWrite(db, outsider.ID, task.ID, 2)
	accessErr, ok := err.(*workflowAccessError)
	if !ok || accessErr.Status != http.StatusForbidden || accessErr.Code != "SCORING_TASK_FORBIDDEN" {
		t.Fatalf("err=%T %+v", err, err)
	}
}

func TestLoadScoringTaskForWriteRejectsVersionConflictAndLockedTask(t *testing.T) {
	db, owner, _, _, task := setupEvaluationAccessDB(t)
	_, err := loadScoringTaskForWrite(db, owner.ID, task.ID, 1)
	accessErr, ok := err.(*workflowAccessError)
	if !ok || accessErr.Status != http.StatusConflict || accessErr.Code != "TASK_VERSION_CONFLICT" {
		t.Fatalf("version err=%T %+v", err, err)
	}
	db.Model(&models.EvaluationScoringTask{}).Where("id = ?", task.ID).Updates(map[string]any{"status": "locked", "version": 2})
	_, err = loadScoringTaskForWrite(db, owner.ID, task.ID, 2)
	accessErr, ok = err.(*workflowAccessError)
	if !ok || accessErr.Code != "TASK_LOCKED" {
		t.Fatalf("locked err=%T %+v", err, err)
	}
}
