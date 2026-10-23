package handlers

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dootask-kpi-server/models"
	"github.com/gin-gonic/gin"
)

func TestAllocationContributionUsesWorkbookThreshold(t *testing.T) {
	for _, test := range []struct {
		name, total, ratio string
		want               float64
	}{
		{name: "below threshold", total: "80", ratio: "0.10", want: 0},
		{name: "threshold inclusive", total: "80", ratio: "0.20", want: 16},
		{name: "round one decimal", total: "84.25", ratio: "0.50", want: 42.1},
	} {
		t.Run(test.name, func(t *testing.T) {
			var total, ratio float64
			if _, err := fmt.Sscanf(test.total, "%f", &total); err != nil {
				t.Fatal(err)
			}
			if _, err := fmt.Sscanf(test.ratio, "%f", &ratio); err != nil {
				t.Fatal(err)
			}
			got, err := allocationContribution(total, ratio, defaultAllocationThreshold)
			if err != nil || got != test.want {
				t.Fatalf("got=%v err=%v want=%v", got, err, test.want)
			}
		})
	}
}

func TestAllocationContributionRejectsInvalidNumbers(t *testing.T) {
	for _, values := range [][3]float64{
		{math.NaN(), 0.5, 0.2},
		{80, math.Inf(1), 0.2},
		{80, 0.5, 1.2},
	} {
		if _, err := allocationContribution(values[0], values[1], values[2]); err == nil {
			t.Fatalf("accepted invalid allocation values: %v", values)
		}
	}
}

func TestPerformanceSummaryUsesPeriodAndWritesDiagnostics(t *testing.T) {
	db := newEvaluationWorkflowTestDB(t)
	models.DB = db
	department := models.Department{Name: "汇总测试部"}
	if err := db.Create(&department).Error; err != nil {
		t.Fatal(err)
	}
	employee := createWorkflowFixtureEmployee(t, db, "汇总员工", "summary-v2@test", "工程师", department.ID)
	assignScope(t, employee.ID, "ALL")
	month := 9
	monthly := models.KPIEvaluation{EmployeeID: employee.ID, Period: "monthly", Year: 2026, Month: &month, Status: "completed", TotalScore: 84.25, CalculationVersion: "legacy-v1"}
	quarter := 3
	quarterly := models.KPIEvaluation{EmployeeID: employee.ID, Period: "quarterly", Year: 2026, Quarter: &quarter, Status: "completed", TotalScore: 90, CalculationVersion: "legacy-v1"}
	if err := db.Create(&monthly).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&quarterly).Error; err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	router.GET("/statistics/performance-summary", func(c *gin.Context) { c.Set("user_id", employee.ID); GetPerformanceSummary(c) })
	request := httptest.NewRequest(http.MethodGet, "/statistics/performance-summary?employee_id="+fmt.Sprint(employee.ID)+"&period_type=monthly&period=2026-09&allocation_ratio=0.5&diagnostics=true", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("summary status=%d body=%s", response.Code, response.Body.String())
	}
	var envelope struct {
		Data struct {
			EvaluationID         uint    `json:"evaluation_id"`
			TotalScore           float64 `json:"total_score"`
			Grade                string  `json:"grade"`
			WeightedContribution float64 `json:"weighted_contribution"`
			DiagnosticLevel      string  `json:"diagnostic_level"`
			CalculationVersion   string  `json:"calculation_version"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.EvaluationID != monthly.ID || envelope.Data.TotalScore != 84.25 || envelope.Data.Grade != "A" || envelope.Data.WeightedContribution != 42.1 || envelope.Data.DiagnosticLevel != "warning" || envelope.Data.CalculationVersion != "legacy-v1" {
		t.Fatalf("unexpected summary: %s", response.Body.String())
	}
	var diagnosed, calculated int64
	db.Model(&models.AuditLog{}).Where("action = ?", "aggregation_diagnosed").Count(&diagnosed)
	db.Model(&models.AuditLog{}).Where("action = ?", "aggregation_calculated").Count(&calculated)
	if diagnosed != 1 || calculated != 1 {
		t.Fatalf("aggregation audit diagnosed=%d calculated=%d", diagnosed, calculated)
	}

	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/statistics/performance-summary?employee_id="+fmt.Sprint(employee.ID)+"&period_type=annual&period=2026", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"evaluation_count":2`) {
		t.Fatalf("annual aggregation status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestPerformanceSummaryAggregatesProjectsWithStoredAllocation(t *testing.T) {
	db := newEvaluationWorkflowTestDB(t)
	models.DB = db
	department := models.Department{Name: "多项目汇总部"}
	if err := db.Create(&department).Error; err != nil {
		t.Fatal(err)
	}
	employee := createWorkflowFixtureEmployee(t, db, "多项目员工", "multi-project@test", "工程师", department.ID)
	assignScope(t, employee.ID, "ALL")
	month := 9
	ratioA, ratioB := 0.5, 0.4
	first := models.KPIEvaluation{EmployeeID: employee.ID, Period: "monthly", Year: 2026, Month: &month, Status: "completed", TotalScore: 80, ProjectName: "项目A", AllocationRatio: &ratioA, CalculationVersion: "legacy-v1"}
	second := models.KPIEvaluation{EmployeeID: employee.ID, Period: "monthly", Year: 2026, Month: &month, Status: "completed", TotalScore: 70, ProjectName: "项目B", AllocationRatio: &ratioB, CalculationVersion: "legacy-v1"}
	if err := db.Create(&first).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&second).Error; err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.GET("/statistics/performance-summary", func(c *gin.Context) { c.Set("user_id", employee.ID); GetPerformanceSummary(c) })
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/statistics/performance-summary?employee_id="+fmt.Sprint(employee.ID)+"&period_type=monthly&period=2026-09", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("multi project summary=%d %s", response.Code, response.Body.String())
	}
	var envelope struct {
		Data performanceSummaryResponse `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.EvaluationCount != 2 || envelope.Data.TotalScore != 75.6 || envelope.Data.WeightedContribution == nil || *envelope.Data.WeightedContribution != 68 || len(envelope.Data.Projects) != 2 {
		t.Fatalf("unexpected multi project summary: %+v", envelope.Data)
	}
}
