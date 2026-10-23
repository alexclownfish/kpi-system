"""Additional boundary and readback checks for an existing isolated acceptance run."""
import io
import json
from decimal import Decimal
from pathlib import Path
from pypdf import PdfReader
from verify_source_workbook import grade, rounded
from verify_workbook_runtime import Acceptance, normalize_pdf


def supplement(output):
    output = Path(output)
    a = Acceptance("http://localhost:18089", output)
    previous = json.loads((output / "results.json").read_text())
    manifest = json.loads((output / "source.json").read_text())
    a.results, a.scenarios = previous["checks"], previous["scenarios"]
    password = "Synthetic-Isolated-Test-20261001!"
    hr = a.login("source-hr@example.test", password)
    outsider = a.login("source-outsider@example.test", password)
    cases = {c["sheet"] + "/" + c["position"]: c for c in manifest["cases"]}
    # Re-read the actual exports after improving glyph equivalence checks.
    for scenario in a.scenarios:
        if scenario["status"] != "workflow_executed":
            continue
        case, eid = cases[scenario["case"]], scenario["evaluation_id"]
        pdf, headers = a.export(eid, "pdf", hr)
        reader = PdfReader(io.BytesIO(pdf))
        text = normalize_pdf("".join(p.extract_text() or "" for p in reader.pages))
        missing = [i["name"] for i in case["items"] if normalize_pdf(i["name"]) not in text]
        portrait = all(float(p.mediabox.height) > float(p.mediabox.width) for p in reader.pages)
        for check in a.results:
            if check["case"] == scenario["case"] and check["name"] == "签字PDF全部页面竖向及完整指标":
                check["pass"] = portrait and not missing and "员工签字" in text and f"{scenario['expected_total']:.2f}" in text
                check["detail"] = {"portrait": portrait, "missing_items": missing, "pages": len(reader.pages), "expected_total": scenario["expected_total"], "normalization": "NFKC + equivalent Chinese radicals + whitespace"}
        signatures = ["项目经理", "CTO", "产品经理"] if case["position"] == "部门负责人" else ["项目经理", "部门负责人", "产品经理"]
        if case["sheet"] == "09-产品部门":
            signatures = ["项目经理", "CTO"]
        a.check(scenario["case"], "签字栏体现原表实际评分人", all(normalize_pdf(s + "签字") in text for s in signatures), {"expected": signatures, "actual_generic": "员工签字、直属主管签字、HR签字"})
    stats = a.required("GET", "/api/statistics/dashboard?period=monthly&year=2026&month=9", hr)["data"]
    actual_totals = [a.required("GET", f"/api/evaluations/{s['evaluation_id']}/result", hr)["data"]["total_score"] for s in a.scenarios if s["status"] == "workflow_executed"]
    mean = sum(actual_totals) / len(actual_totals)
    a.check("统计", "同周期看板数据", abs(stats["average_score"] - mean) < 0.000001 and stats["total_evaluations"] == 15 and stats["completed_evaluations"] == 15, {"dashboard": stats, "canonical_mean": mean})
    trends = a.required("GET", "/api/statistics/trends?period=monthly&year=2026", hr)["data"]
    periods = [p["period"] for p in trends["score_trends"]]
    a.check("统计", "月份无重复且有序", len(periods) == len(set(periods)) and periods == sorted(periods), periods)
    first = next(s for s in a.scenarios if s["case"] == "01-ID工业设计/工程师")
    source_template = a.required("GET", f"/api/templates/{first['template_id']}", hr)["data"]
    owner_email = first["owner_email"]
    owner = a.login(owner_email, password)
    eid = first["evaluation_id"]
    assessment = a.required("GET", f"/api/evaluations/{eid}", hr)["data"]
    status, _, _ = a.call("GET", f"/api/evaluations/{eid}/comments", outsider)
    a.check("权限", "无关用户不能读取考核评论", status in (403, 404), status)
    clone = a.required("POST", f"/api/templates/{source_template['id']}/clone", hr, {"name": "原表ID工程师边界验收"}, 201)["data"]
    a.required("POST", f"/api/templates/{clone['id']}/publish", hr, {})
    for month, score in enumerate([0, 59, 60, 69, 70, 79, 80, 89, 90, 100], 1):
        evaluation = a.required("POST", "/api/evaluations", hr, {"employee_id": assessment["employee_id"], "period": "monthly", "year": 2025, "month": month}, 201)["data"]
        eid = evaluation["id"]
        for self_score in a.required("GET", f"/api/scores/evaluation/{eid}", owner)["data"]:
            a.required("PUT", f"/api/scores/{self_score['id']}/self", owner, {"self_score": 100})
        a.required("POST", f"/api/evaluations/{eid}/workflow/advance", owner, {})
        tasks = a.required("GET", f"/api/evaluations/{eid}/scoring-tasks", hr)["data"]["tasks"]
        users = {"PM": "source-pm-0@example.test", "DEPARTMENT_MANAGER": "source-head-0@example.test", "PRODUCT_MANAGER": "source-product-0@example.test"}
        for task in tasks:
            token = a.login(users[task["scorer_type"]], password)
            a.required("POST", f"/api/scoring-tasks/{task['id']}/submit", token, {"score": score, "version": task["version"]})
        a.required("POST", f"/api/evaluations/{eid}/workflow/advance", hr, {})
        result = a.required("GET", f"/api/evaluations/{eid}/result", hr)["data"]
        a.check("等级边界", f"全部指标{score}分", result["total_score"] == score and result["grade"] == grade(score), {"total": result["total_score"], "grade": result["grade"], "self_score": 100})
        status, _, _ = a.call("POST", f"/api/scoring-tasks/{tasks[0]['id']}/submit", a.login("source-pm-0@example.test", password), {"score": 1, "version": tasks[0]["version"]})
        a.check("等级边界", f"提交后不可覆盖{score}分", status == 409, status)
        a.required("POST", f"/api/evaluations/{eid}/confirm-online", owner, {})
    # Source HR examples and thresholds cannot pass without persisted project/effort fields.
    expected = [{"effort": str(e), "expected_contribution": float(rounded(Decimal(80) * e if e >= Decimal("0.2") else 0, 1))} for e in [Decimal("0.1999"), Decimal("0.2"), Decimal("0.5")]]
    a.check("HR汇总", "原表投入占比阈值和项目加权贡献", False, {"source_examples": {"monthly!J5": 40, "monthly!J6": 28}, "boundary_oracles": expected, "reason": "No project_name/effort_ratio/weighted_contribution fields or HR period aggregation endpoint in current model/routes; 006 T037-T042 incomplete."})
    monthly = rounded((Decimal(80) + Decimal(70) * 11) / 12, 1)
    quarterly = rounded((Decimal(80) + Decimal(70) * Decimal("1.3") + Decimal(90) * Decimal("1.5") + Decimal(60) * Decimal("1.3")) / Decimal("5.1"), 1)
    a.check("HR汇总", "原表年度折算分数与等级", False, {"monthly_source_sample_expected": float(monthly), "grade": grade(monthly), "quarterly_synthetic_expected": float(quarterly), "quarter_weights": [1, 1.3, 1.5, 1.3], "reason": "Configurable period weights and missing-period weighted-average feature unimplemented; quarterly source formulas in K/L instead of H/I require explicit correction."})
    pending = a.required("POST", "/api/evaluations", hr, {"employee_id": assessment["employee_id"], "period": "monthly", "year": 2026, "month": 10}, 201)["data"]
    summary = {"passed": sum(c["pass"] for c in a.results), "failed": sum(not c["pass"] for c in a.results), "total": len(a.results), "workflow_executed": 15, "source_blocked": 3, "grade_boundary_workflows": 10}
    (output / "final-results.json").write_text(json.dumps({"source_sha256": manifest["sha256"], "scenarios": a.scenarios, "checks": a.results, "summary": summary, "pending_ui_evaluation": pending["id"]}, ensure_ascii=False, indent=2))
    print("SUMMARY", summary, "UI_PENDING", pending["id"])
    return summary


if __name__ == "__main__":
    import sys
    summary = supplement(sys.argv[1])
    raise SystemExit(1 if summary["failed"] else 0)
