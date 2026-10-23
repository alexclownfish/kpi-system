"""Run source-workbook acceptance against a disposable, isolated backend only."""
import argparse
from decimal import Decimal
import io
import json
import re
import unicodedata
import urllib.error
import urllib.request
import zipfile
from pathlib import Path
from pypdf import PdfReader
import openpyxl

from verify_source_workbook import extract, grade, rounded


def normalize_pdf(text):
    # The embedded Chinese font maps a few glyphs to radical code points.
    equivalents = str.maketrans({"⻔": "门", "⻅": "见", "⻋": "车", "⻓": "长", "⻛": "风", "⻢": "马", "⻚": "页", "⻬": "齐", "戶": "户"})
    return re.sub(r"\s+", "", unicodedata.normalize("NFKC", text).translate(equivalents))


class Acceptance:
    def __init__(self, base, output):
        if base != "http://localhost:18089":
            raise ValueError("Only the isolated acceptance backend at localhost:18089 is allowed")
        self.base, self.output = base, Path(output)
        self.output.mkdir(parents=True, exist_ok=True)
        self.results, self.scenarios = [], []

    def call(self, method, path, token=None, payload=None, raw=None, content_type=None):
        headers = {}
        if token:
            headers["Authorization"] = "Bearer " + token
        if payload is not None:
            raw = json.dumps(payload).encode()
            headers["Content-Type"] = "application/json"
        if content_type:
            headers["Content-Type"] = content_type
        request = urllib.request.Request(self.base + path, data=raw, headers=headers, method=method)
        try:
            response = urllib.request.urlopen(request, timeout=90)
        except urllib.error.HTTPError as error:
            response = error
        body = response.read()
        return response.status, json.loads(body) if "json" in response.headers.get("Content-Type", "") else body, response.headers

    def check(self, case, name, ok, detail):
        result = {"case": case, "name": name, "pass": bool(ok), "detail": detail}
        self.results.append(result)
        print(("PASS " if ok else "FAIL ") + case + " / " + name + ": " + str(detail), flush=True)

    def required(self, method, path, token, payload=None, expected=200):
        status, body, _ = self.call(method, path, token, payload)
        if status != expected:
            raise RuntimeError(f"{method} {path}: {status} {body}")
        return body

    @staticmethod
    def test_fixture(source_case, template_index):
        """Create scores only for this disposable acceptance run.

        The workbook parser must preserve blank source scores.  The runtime
        test still needs concrete inputs to exercise the full workflow, so
        those inputs are generated here and never written to the catalog.
        """
        case = dict(source_case)
        items = []
        total = Decimal("0")
        for item_index, source_item in enumerate(source_case["items"]):
            item = dict(source_item)
            score = (template_index * 13 + item_index * 11) % 101
            contribution = rounded(Decimal(str(item["composite_weight"])) * Decimal(score) / 100, 2)
            item["score"] = score
            item["contribution"] = float(contribution)
            items.append(item)
            total += contribution
        case["items"] = items
        case["expected_total"] = float(total)
        case["expected_grade"] = grade(total)
        case["score_complete"] = True
        return case

    def login(self, email, password):
        return self.required("POST", "/api/auth/login", None, {"email": email, "password": password})["token"]

    def export(self, eid, fmt, token):
        body = self.required("GET", f"/api/export/evaluation/{eid}?format={fmt}", token)
        url = body["file_url"]
        if not url.startswith(self.base + "/"):
            raise RuntimeError("Unexpected download host: " + url)
        status, data, headers = self.call("GET", url[len(self.base):])
        if status != 200:
            raise RuntimeError("Download failed")
        (self.output / f"evaluation-{eid}.{fmt}").write_bytes(data)
        return data, headers

    def run(self, manifest):
        password = "Synthetic-Isolated-Test-20261001!"
        admin = self.login("source-admin@example.test", password)
        departments = {}
        for case in manifest["cases"]:
            if case["department"] not in departments:
                departments[case["department"]] = self.required("POST", "/api/departments", admin,
                    {"name": case["department"], "description": "原表隔离验收"}, 201)["data"]["id"]
        def employee(name, email, role, department, position, manager=None):
            payload = {"name": name, "email": email, "password": password, "role": role,
                       "department_id": department, "position": position, "is_active": True}
            if manager:
                payload["manager_id"] = manager
            data = self.required("POST", "/api/employees", admin, payload, 201)["data"]
            return {"id": data["id"], "token": self.login(email, password), "email": email}
        first_department = next(iter(departments.values()))
        hr = employee("原表验收HR", "source-hr@example.test", "hr_admin", first_department, "HR")
        cto = employee("原表验收CTO", "source-cto@example.test", "cto", first_department, "CTO")
        for index, (department, did) in enumerate(departments.items()):
            head = employee(department + "负责人", f"source-head-{index}@example.test", "department_manager", did, "部门负责人")
            engineer = employee(department + "工程师", f"source-engineer-{index}@example.test", "employee", did, "工程师", head["id"])
            # Ordinary employee RBAC demonstrates that responsibilities, not role names, assign PM work.
            pm = employee(department + "项目经理", f"source-pm-{index}@example.test", "employee", did, "项目经理", head["id"])
            product = employee(department + "产品经理", f"source-product-{index}@example.test", "product_manager", did, "产品经理")
            departments[department] = {"id": did, "head": head, "engineer": engineer, "pm": pm, "product": product}
            for code, user in [("PM", pm), ("DEPARTMENT_MANAGER", head), ("PRODUCT_MANAGER", product), ("CTO", cto)]:
                self.required("POST", "/api/evaluation-responsibilities", hr["token"],
                    {"code": code, "name": "原表验收" + code, "scope_type": "department", "department_id": did, "user_id": user["id"]}, 201)
        outsider = employee("无关员工", "source-outsider@example.test", "employee", first_department, "无关岗位", departments[next(iter(departments))]["head"]["id"])
        first = departments[next(iter(departments))]
        status, body, _ = self.call("POST", "/api/employees", hr["token"],
            {"name": "HR创建验收员工", "email": "source-hr-created@example.test", "password": password,
             "role": "employee", "position": "工程师", "department_id": first["id"], "manager_id": first["head"]["id"], "is_active": True})
        self.check("组织", "HR正常创建员工", status == 201, {"status": status, "body": body.get("error", body.get("message"))})
        completed = []
        for index, source_case in enumerate(manifest["cases"]):
            case = self.test_fixture(source_case, index)
            label = case["sheet"] + "/" + case["position"]
            scenario = {"case": label, "expected_total": case["expected_total"], "expected_grade": case["expected_grade"], "source_issues": case["source_issues"]}
            self.scenarios.append(scenario)
            try:
                department = departments[case["department"]]
                owner = department["head" if case["position"] == "部门负责人" else "engineer"]
                template = self.required("POST", "/api/templates", hr["token"],
                    {"name": label, "period": "monthly", "department_id": department["id"], "position_type": case["position"]}, 201)["data"]
                tid = template["id"]
                status, structure, _ = self.call("PUT", f"/api/templates/{tid}/calculation-structure", hr["token"], {"dimensions": case["dimensions"]})
                if case["composite_sum"] != 100:
                    self.check(label, "原表综合权重不为100被拒绝", status == 400, {"status": status, "body": structure})
                    scenario["status"] = "blocked_composite_weights"
                    continue
                self.check(label, "原表完整指标结构保存", status == 200, status)
                if status != 200:
                    raise RuntimeError(str(structure))
                template = structure["data"]
                if case["source_issues"]:
                    self.check(label, "保留原表内部权重不归一化", all(
                        actual["item_weight"] == next(i["item_weight"] for i in case["items"] if i["name"] == actual["name"])
                        for actual in template["items"]), case["source_issues"])
                expected_by_name = {i["name"]: i for i in case["items"]}
                actual_items = template["items"]
                self.check(label, "综合权重及评分标准与原表一致", len(actual_items) == len(case["items"]) and all(
                    i["composite_weight"] == expected_by_name[i["name"]]["composite_weight"] and
                    i["scoring_guidance"] == expected_by_name[i["name"]]["scoring_guidance"] for i in actual_items), len(actual_items))
                self.required("POST", f"/api/templates/{tid}/publish", hr["token"], {})
                status, _, _ = self.call("PUT", f"/api/templates/{tid}", hr["token"], {"name": "篡改已发布模板"})
                self.check(label, "已发布模板不可覆盖", status in (400, 409, 422), status)
                evaluation = self.required("POST", "/api/evaluations", hr["token"],
                    {"employee_id": owner["id"], "period": "monthly", "year": 2026, "month": 9}, 201)["data"]
                eid = evaluation["id"]
                scenario.update(evaluation_id=eid, template_id=tid, owner_email=owner["email"])
                self.check(label, "按部门岗位自动匹配", evaluation["template_id"] == tid and evaluation["status"] == "self_evaluating", evaluation["status"])
                status, _, _ = self.call("POST", f"/api/evaluations/{eid}/workflow/advance", owner["token"], {})
                self.check(label, "缺少必填自评不能推进", status == 422, status)
                scores = self.required("GET", f"/api/scores/evaluation/{eid}", hr["token"])["data"]
                if case["position"] == "部门负责人":
                    status, body, _ = self.call("PUT", f"/api/scores/{scores[0]['id']}/self", owner["token"], {"self_score": 0})
                    self.check(label, "模板已有评分职责时无需旧直属上级才能自评", status == 200, {"status": status, "body": body.get("error")})
                    if status != 200:
                        current_owner = self.required("GET", f"/api/employees/{owner['id']}", admin)["data"]
                        current_owner["manager_id"] = cto["id"]
                        self.required("PUT", f"/api/employees/{owner['id']}", admin, current_owner)
                        scenario["fixture_condition"] = "No-manager self-score failed; synthetic manager assigned only to continue other checks."
                status, _, _ = self.call("PUT", f"/api/scores/{scores[0]['id']}/self", outsider["token"], {"self_score": 99, "self_comment": "模拟越权验证"})
                self.check(label, "无关员工不能写本人自评", status in (403, 404), status)
                for score in scores:
                    self.required("PUT", f"/api/scores/{score['id']}/self", owner["token"], {"self_score": 0, "self_comment": "测试自评零分不计总分"})
                self.required("POST", f"/api/evaluations/{eid}/workflow/advance", owner["token"], {})
                tasks = self.required("GET", f"/api/evaluations/{eid}/scoring-tasks", hr["token"])["data"]["tasks"]
                users = {"PM": department["pm"], "DEPARTMENT_MANAGER": department["head"], "PRODUCT_MANAGER": department["product"], "CTO": cto}
                self.check(label, "实际评分人逐指标与原表一致", len(tasks) == len(case["items"]) and all(
                    t["scorer_user_id"] == users[t["scorer_type"]]["id"] for t in tasks), len(tasks))
                pm_tasks = self.required("GET", f"/api/evaluations/{eid}/scoring-tasks", department["pm"]["token"])["data"]["tasks"]
                self.check(label, "PM任务页面仅返回本人负责指标", len(pm_tasks) == sum(t["scorer_type"] == "PM" for t in tasks) and
                    all(t["scorer_user_id"] == department["pm"]["id"] for t in pm_tasks), len(pm_tasks))
                pm_list = self.required("GET", "/api/evaluations", department["pm"]["token"])["data"]
                self.check(label, "普通员工PM能在考核列表找到任务", any(e["id"] == eid for e in pm_list), [e["id"] for e in pm_list])
                first_task = tasks[0]
                for invalid, expected_status in [(101, 400), (-1, 400), (None, 422)]:
                    status, _, _ = self.call("POST", f"/api/scoring-tasks/{first_task['id']}/submit", users[first_task["scorer_type"]]["token"], {"score": invalid, "version": first_task["version"]})
                    self.check(label, "拒绝无效评分" + str(invalid), status == expected_status, status)
                status, _, _ = self.call("POST", f"/api/scoring-tasks/{first_task['id']}/submit", outsider["token"], {"score": 99, "version": first_task["version"]})
                self.check(label, "无关用户不能提交评分", status in (403, 404), status)
                status, _, _ = self.call("POST", f"/api/evaluations/{eid}/workflow/advance", hr["token"], {})
                self.check(label, "必填未评分不能进入确认", status == 422, status)
                task_by_item = {i["id"]: expected_by_name[i["name"]] for i in actual_items}
                for task in tasks:
                    source_item = task_by_item[task["item_id"]]
                    user = users[task["scorer_type"]]
                    self.required("POST", f"/api/scoring-tasks/{task['id']}/submit", user["token"],
                        {"score": source_item["score"], "version": task["version"], "comment": "原表验收模拟输入"})
                self.required("POST", f"/api/evaluations/{eid}/workflow/advance", hr["token"], {})
                result = self.required("GET", f"/api/evaluations/{eid}/result", hr["token"])["data"]
                self.check(label, "原表公式逐项贡献及总分等级一致", result["total_score"] == case["expected_total"] and result["grade"] == case["expected_grade"] and all(
                    item["contribution"] == expected_by_name[item["name"]]["contribution"] for item in result["items"]),
                    {"actual": result["total_score"], "expected": case["expected_total"], "grade": result["grade"]})
                pdf, pdf_headers = self.export(eid, "pdf", hr["token"])
                reader = PdfReader(io.BytesIO(pdf))
                text = normalize_pdf("".join(page.extract_text() or "" for page in reader.pages))
                self.check(label, "签字PDF全部页面竖向及完整指标", all(float(p.mediabox.height) > float(p.mediabox.width) for p in reader.pages) and
                    all(normalize_pdf(i["name"]) in text for i in case["items"]) and "员工签字" in text and f"{case['expected_total']:.2f}" in text,
                    {"pages": len(reader.pages), "mime": pdf_headers.get("Content-Type")})
                xlsx, _ = self.export(eid, "xlsx", hr["token"])
                sheet = openpyxl.load_workbook(io.BytesIO(xlsx), data_only=True).active
                values = [cell.value for row in sheet for cell in row if cell.value is not None]
                excel_text = " ".join(str(v) for v in values)
                self.check(label, "Excel指标总分及签字字段一致", all(i["name"] in excel_text for i in case["items"]) and
                    "员工签字" in excel_text and f"{case['expected_total']:.2f}" in excel_text, {"rows": sheet.max_row})
                if label.startswith("01-") and case["position"] == "部门负责人":
                    self.required("POST", f"/api/evaluations/{eid}/objection", owner["token"], {"reason": "验收异议，不修改模拟得分"})
                    status, _, _ = self.call("POST", f"/api/evaluations/{eid}/confirm-online", owner["token"], {})
                    self.check(label, "未处理异议阻止确认", status in (400, 409, 422), status)
                    self.required("PUT", f"/api/evaluations/{eid}/objection/handle", hr["token"], {"total_score": 999, "final_comment": "验收复核，原评分有效"})
                    new_result = self.required("GET", f"/api/evaluations/{eid}/result", hr["token"])["data"]
                    self.check(label, "异议复核不能伪造总分", new_result["total_score"] == case["expected_total"], new_result["total_score"])
                if label.startswith("08-") and case["position"] == "工程师":
                    pdf, _ = self.export(eid, "pdf", hr["token"])
                    boundary = "SourceAcceptanceBoundary"
                    raw = b""
                    for field, value in [("signed_at", "2026-10-01"), ("remark", "模拟签字附件，仅用于验收")]:
                        raw += f'--{boundary}\r\nContent-Disposition: form-data; name="{field}"\r\n\r\n{value}\r\n'.encode()
                    raw += f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="synthetic.pdf"\r\nContent-Type: application/pdf\r\n\r\n'.encode() + pdf + f"\r\n--{boundary}--\r\n".encode()
                    status, confirmation, _ = self.call("POST", f"/api/evaluations/{eid}/confirm-paper", hr["token"], raw=raw, content_type="multipart/form-data; boundary=" + boundary)
                    self.check(label, "模拟纸质签字登记完成", status == 200 and confirmation.get("evaluation", {}).get("status") == "completed", status)
                    status, attachment, _ = self.call("GET", f"/api/evaluations/{eid}/confirmation/attachment", owner["token"])
                    self.check(label, "签字附件完整下载", status == 200 and attachment == pdf, status)
                else:
                    status, confirmation, _ = self.call("POST", f"/api/evaluations/{eid}/confirm-online", owner["token"], {})
                    self.check(label, "员工确认完成", status == 200 and confirmation.get("evaluation", {}).get("status") == "completed", {"status": status, "body": confirmation.get("error")})
                frozen = self.required("GET", f"/api/evaluations/{eid}/result", hr["token"])["data"]
                clone = self.required("POST", f"/api/templates/{tid}/clone", hr["token"], {"name": label + "维护新版本"}, 201)["data"]
                self.required("PUT", f"/api/templates/{clone['id']}", hr["token"], {"description": "HR后续维护"})
                self.required("POST", f"/api/templates/{tid}/retire", hr["token"], {})
                current = self.required("GET", f"/api/evaluations/{eid}/result", hr["token"])["data"]
                self.check(label, "HR维护新版本历史结果不变", frozen == current, current["snapshot_checksum"])
                scenario["status"] = "workflow_executed"
                completed.append((eid, case, pdf))
            except Exception as error:
                scenario["status"] = "blocked_runtime"
                self.check(label, "完整流程执行", False, str(error))
        if completed:
            body = self.required("GET", "/api/export/signoff-batch?period=monthly&year=2026&month=9", hr["token"])
            status, data, _ = self.call("GET", body["file_url"][len(self.base):])
            (self.output / "batch.zip").write_bytes(data)
            with zipfile.ZipFile(io.BytesIO(data)) as archive:
                pdfs = [archive.read(n) for n in archive.namelist() if n.endswith(".pdf")]
                batch_text = ["".join(p.extract_text() or "" for p in PdfReader(io.BytesIO(pdf)).pages) for pdf in pdfs]
                self.check("批量", "所有原表流程PDF均收入批次", len(pdfs) == len(completed), len(pdfs))
                for eid, case, pdf in completed:
                    # Compare latest single export after confirmation to the same batch snapshot.
                    latest, _ = self.export(eid, "pdf", hr["token"])
                    single_text = "".join(p.extract_text() or "" for p in PdfReader(io.BytesIO(latest)).pages)
                    self.check(case["sheet"] + "/" + case["position"], "单份与批量PDF完整文本一致", single_text in batch_text, eid)
            for eid, case, _ in completed:
                stats = self.required("GET", f"/api/statistics/employee/{self.required('GET', f'/api/evaluations/{eid}', hr['token'])['data']['employee_id']}", hr["token"])
                actual = stats["data"]
                self.check(case["sheet"] + "/" + case["position"], "个人报表使用权威总分", actual["latest_score"] == case["expected_total"] and actual["average_score"] == case["expected_total"], {"latest": actual["latest_score"], "average": actual["average_score"]})
                source_items = {i["name"]: i for i in case["items"]}
                self.check(case["sheet"] + "/" + case["position"], "个人报表逐项分数与评分结果一致", all(i["average_score"] == source_items[i["item_name"]]["score"] for i in actual["kpi_breakdown"]), actual["kpi_breakdown"])
        self.output.joinpath("results.json").write_text(json.dumps({"source_sha256": manifest["sha256"], "scenarios": self.scenarios,
            "checks": self.results, "summary": {"passed": sum(r["pass"] for r in self.results), "failed": sum(not r["pass"] for r in self.results),
                "workflow_executed": len(completed), "source_blocked": sum(s["status"] == "blocked_composite_weights" for s in self.scenarios)}}, ensure_ascii=False, indent=2))
        print("SUMMARY", len(self.results), sum(r["pass"] for r in self.results), "passed;", len(completed), "workflows", flush=True)


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("workbook")
    parser.add_argument("--base", default="http://localhost:18089")
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    manifest = extract(args.workbook)
    acceptance = Acceptance(args.base, args.output)
    acceptance.output.joinpath("source.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2))
    acceptance.run(manifest)
    raise SystemExit(1 if any(not check["pass"] for check in acceptance.results) else 0)
