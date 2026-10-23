"""Regression checks for the versioned Hongxing source workbook parser.

This is the command named by Spec-Kit feature 013. It intentionally performs
only read-only checks against the workbook supplied on the command line.
"""

import argparse
import json
import sys
import tempfile
from pathlib import Path

import openpyxl

sys.path.insert(0, str(Path(__file__).resolve().parent))
from build_hongxing_catalog import build  # noqa: E402
from verify_source_workbook import extract  # noqa: E402


def check(workbook: str) -> None:
    source = extract(workbook)
    catalog = build(workbook)
    manifest_path = Path(__file__).resolve().parent / "testdata" / "hongxing-workbook-v2-manifest.json"
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    assert source["sha256"] == manifest["source_sha256"]
    assert source["template_sheets"] == manifest["template_sheets"]
    assert set(source["source_departments"]) == set(manifest["source_departments"])
    assert source["workbook_sheet_count"] == 21, source["workbook_sheet_count"]
    assert source["template_sheet_count"] == 18, source["template_sheet_count"]
    assert sum(len(case["items"]) for case in source["cases"]) == 137
    assert source["source_department_count"] == 9
    all_items = [item for case in source["cases"] for item in case["items"]]
    assert all(item["score_origin"] in {"source", "blank"} for item in all_items)
    assert all(item["score_origin"] != "synthetic" for item in all_items)
    assert any(item["score_origin"] == "blank" and item["score"] is None for item in all_items)
    assert all(item["score"] is None or 0 <= item["score"] <= 100 for item in all_items)
    assert any(case["score_complete"] is False and case["expected_total"] is None for case in source["cases"])
    assert catalog["version"] == "hongxing-catalog-v2"
    assert len(catalog["profiles"]) == 18
    assert all(case["composite_sum"] == 100 for case in source["cases"])
    quarter = next(item for item in source["hr"] if item["kind"] == "quarterly")
    assert {gap["cell"] for gap in quarter["formula_gaps"]} == {"I17", "J17"}
    assert all(gap["severity"] == "warning" and not gap["is_real_data_row"] for gap in quarter["formula_gaps"])
    assert all(gap["input_cells"] == [] for gap in quarter["formula_gaps"])
    catalog_quarter = next(item for item in catalog["hr_summary_diagnostics"] if item["kind"] == "quarterly")
    assert catalog_quarter["formula_gaps"] == quarter["formula_gaps"]
    assert len(quarter["annual_formula_issues"]) == 10
    monthly = next(item for item in source["hr"] if item["kind"] == "monthly")
    assert any(group["range"] == "I5:I33" for group in monthly["shared_formula_groups"])
    assert monthly["cells"]["I6"]["source_formula"] is None
    assert monthly["cells"]["I6"]["expanded_formula"] == monthly["cells"]["I5"]["formula"].replace("5", "6")
    assert quarter["cells"]["K40"]["formula_status"] == "formula"
    assert "$D$39:$J$39" in quarter["cells"]["K40"]["formula"]
    # Reorder the sheets in a disposable copy to ensure detection is based on
    # sheet content rather than fixed positions.
    with tempfile.TemporaryDirectory(prefix="hongxing-parser-") as temp:
        reordered = Path(temp) / "reordered.xlsx"
        workbook_copy = openpyxl.load_workbook(workbook, data_only=False)
        workbook_copy._sheets = [workbook_copy["HR汇总（季度）"], workbook_copy["项目绩效考核流程"]] + [
            sheet for sheet in workbook_copy.worksheets
            if sheet.title not in {"HR汇总（季度）", "项目绩效考核流程"}
        ]
        workbook_copy.save(reordered)
        shuffled = extract(reordered)
        assert shuffled["template_sheet_count"] == 18
        assert {item["sheet"] for item in shuffled["hr"]} == {"HR汇总（月度）", "HR汇总（季度）"}

        # The original quarterly row 17 contains only its sequence number.
        # Missing formulas remain visible, but only actual business inputs
        # justify blocking use of an assessment's calculated results.
        populated = Path(temp) / "populated-missing-formulas.xlsx"
        quarterly = workbook_copy["HR汇总（季度）"]
        quarterly["B17"] = "验收员工"
        quarterly["G17"] = 0
        quarterly["H17"] = 0
        workbook_copy.save(populated)
        populated_report = extract(populated)
        populated_hr = next(item for item in populated_report["hr"] if item["kind"] == "quarterly")
        populated_gaps = [gap for gap in populated_hr["formula_gaps"] if gap["row"] == 17]
        assert {gap["cell"] for gap in populated_gaps} == {"I17", "J17"}
        assert all(gap["severity"] == "blocking" and gap["is_real_data_row"] for gap in populated_gaps)
        assert all(gap["input_cells"] == ["B17", "G17", "H17"] for gap in populated_gaps)

        # A real score of zero still requires formulas even with no sequence
        # number, while whitespace in otherwise unused numbered rows does not.
        quarterly["A17"] = None
        quarterly["B17"] = "   "
        quarterly["G17"] = None
        quarterly["H17"] = 0
        workbook_copy.save(populated)
        zero_hr = next(item for item in extract(populated)["hr"] if item["kind"] == "quarterly")
        assert {gap["cell"] for gap in zero_hr["formula_gaps"] if gap["row"] == 17} == {"I17", "J17"}
        assert all(gap["severity"] == "blocking" and gap["is_real_data_row"]
                   and gap["input_cells"] == ["H17"] for gap in zero_hr["formula_gaps"] if gap["row"] == 17)
        quarterly["A17"] = 13
        quarterly["H17"] = None
        workbook_copy.save(populated)
        blank_hr = next(item for item in extract(populated)["hr"] if item["kind"] == "quarterly")
        assert {gap["cell"] for gap in blank_hr["formula_gaps"] if gap["row"] == 17} == {"I17", "J17"}
        assert all(gap["severity"] == "warning" and not gap["is_real_data_row"]
                   for gap in blank_hr["formula_gaps"] if gap["row"] == 17)
    print("PASS Hongxing parser: 21 sheets, 18 templates, 137 indicators, 9 departments")
    print("PASS shared formulas expanded; empty numbered HR gaps warn and populated HR gaps block")


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("workbook")
    args = parser.parse_args()
    check(args.workbook)
