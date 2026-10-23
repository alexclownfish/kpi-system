package handlers

import (
	"errors"
	"strings"
	"testing"

	"dootask-kpi-server/models"
)

func TestResolveTemplateResponsibilitiesUsesBusinessAssignmentsAndScopes(t *testing.T) {
	db := newEvaluationWorkflowTestDB(t)
	department := models.Department{Name: "机器人研发部"}
	if err := db.Create(&department).Error; err != nil {
		t.Fatal(err)
	}
	child := models.Department{Name: "运控部", ParentID: &department.ID}
	if err := db.Create(&child).Error; err != nil {
		t.Fatal(err)
	}
	target := createWorkflowFixtureEmployee(t, db, "普通员工", "resolver-target@test", "工程师", child.ID)
	pm := createWorkflowFixtureEmployee(t, db, "项目经理", "resolver-pm@test", "任意岗位", department.ID)
	product := createWorkflowFixtureEmployee(t, db, "产品经理", "resolver-product@test", "任意岗位", department.ID)
	cto := createWorkflowFixtureEmployee(t, db, "CTO", "resolver-cto@test", "任意岗位", department.ID)
	// These responsibilities are deliberately independent of RBAC roles. The
	// three users remain ordinary employees and still resolve as business
	// scorers.
	createWorkflowFixtureResponsibility(t, db, "PM", "项目经理", "company", pm.ID, target.ID, nil, "")
	createWorkflowFixtureResponsibility(t, db, "PRODUCT_MANAGER", "产品经理", "company", product.ID, target.ID, nil, "")
	createWorkflowFixtureResponsibility(t, db, "CTO", "CTO", "company", cto.ID, target.ID, nil, "")
	// An assignment outside the target's position/departments is ignored.
	outOfScope := createWorkflowFixtureEmployee(t, db, "外部PM", "resolver-outside@test", "任意岗位", 999)
	createWorkflowFixtureResponsibility(t, db, "PM", "外部PM", "department", outOfScope.ID, target.ID, &department.ID, "")
	template := models.KPITemplate{ID: 1, Name: "研发模板", PositionType: "工程师", Period: "monthly", Items: []models.KPIItem{{ID: 1, Name: "交付", MaxScore: 100, Required: true, ScorerTypes: []string{"PM", "PRODUCT_MANAGER", "CTO"}}}}
	assignments, err := resolveTemplateScoringAssignments(db, target, template)
	if err != nil {
		t.Fatal(err)
	}
	if len(assignments) != 3 {
		t.Fatalf("expected three scoped business assignments, got %+v", assignments)
	}
	for _, assignment := range assignments {
		if assignment.UserID == outOfScope.ID {
			t.Fatal("out-of-scope responsibility leaked into assignment")
		}
	}
}

func TestResolveTemplateResponsibilitiesBlocksInactiveAndMissingRequiredScorers(t *testing.T) {
	db := newEvaluationWorkflowTestDB(t)
	department := models.Department{Name: "软件部"}
	if err := db.Create(&department).Error; err != nil {
		t.Fatal(err)
	}
	target := createWorkflowFixtureEmployee(t, db, "目标", "resolver-missing-target@test", "工程师", department.ID)
	pm := createWorkflowFixtureEmployee(t, db, "PM", "resolver-inactive-pm@test", "普通员工", department.ID)
	createWorkflowFixtureResponsibility(t, db, "PM", "项目经理", "department", pm.ID, target.ID, &department.ID, "")
	template := models.KPITemplate{ID: 1, Name: "缺失职责", PositionType: "工程师", Period: "monthly", Items: []models.KPIItem{{ID: 1, Name: "交付", MaxScore: 100, Required: true, ScorerTypes: []string{"PM", "CTO"}}}}
	if err := db.Model(&pm).Update("is_active", false).Error; err != nil {
		t.Fatal(err)
	}
	_, err := resolveTemplateScoringAssignments(db, target, template)
	if err == nil || !strings.Contains(err.Error(), "评分人") {
		t.Fatalf("expected stable missing/inactive responsibility error, got %v", err)
	}
	var structured *scorerResolutionError
	if !errors.As(err, &structured) {
		t.Fatalf("expected structured scorer resolution error, got %T", err)
	}
	if len(structured.Missing) < 2 {
		t.Fatalf("missing details=%v", structured.Missing)
	}
}

func TestResolveDepartmentHeadUsesParentLeaderOrCTOAndRejectsCycle(t *testing.T) {
	db := newEvaluationWorkflowTestDB(t)
	parent := models.Department{Name: "父部门"}
	if err := db.Create(&parent).Error; err != nil {
		t.Fatal(err)
	}
	child := models.Department{Name: "子部门", ParentID: &parent.ID}
	if err := db.Create(&child).Error; err != nil {
		t.Fatal(err)
	}
	parentLeader := createWorkflowFixtureEmployee(t, db, "父负责人", "resolver-parent@test", "普通员工", parent.ID)
	head := createWorkflowFixtureEmployee(t, db, "子负责人", "resolver-head@test", "部门负责人", child.ID)
	if err := db.Model(&parent).Update("leader_id", parentLeader.ID).Error; err != nil {
		t.Fatal(err)
	}
	users, source, err := resolveScorerUsersByResponsibility(db, head, "DEPARTMENT_MANAGER")
	if err != nil || len(users) != 1 || users[0].ID != parentLeader.ID || source != "PARENT_DEPARTMENT_LEADER" {
		t.Fatalf("parent leader resolution got users=%+v source=%s err=%v", users, source, err)
	}
	if err := db.Model(&parentLeader).Update("manager_id", head.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := resolveScorerUsersByResponsibility(db, head, "DEPARTMENT_MANAGER"); err == nil {
		t.Fatal("expected manager cycle to block department-head resolution")
	}
}
