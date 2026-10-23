"""Build the source-backed Hongxing catalog v2; never modify the workbook."""

import argparse
import json
from pathlib import Path

from verify_source_workbook import extract


def build(path: str | Path) -> dict:
    source = extract(path)
    profiles = []
    for case in source["cases"]:
        dimensions = []
        for dimension in case["dimensions"]:
            items = [{key: item[key] for key in (
                "name", "item_weight", "scorer_types", "required", "scoring_guidance", "data_source",
                "order", "source_row", "source_scorer_label", "source_score_cell",
            )} for item in dimension["items"]]
            dimensions.append({"name": dimension["name"], "weight": dimension["weight"],
                               "sort_order": dimension["sort_order"], "items": items})
        profiles.append({"sheet": case["sheet"], "department": case["department"], "position": case["position"],
                         "level": case["level"], "period": "monthly", "dimensions": dimensions,
                         "source_metadata": case["source_metadata"],
                         "source_anomalies": case["source_issues"] + case["formula_issues"] + case["metadata_issues"]})
    return {"version": "hongxing-catalog-v2", "source_sha256": source["sha256"],
            "source_filename": source["source_filename"], "source_sheet_count": source["workbook_sheet_count"],
            "source_template_count": source["template_sheet_count"], "source_department_count": source["source_department_count"],
            "source_departments": source["source_departments"], "flow": source["flow"],
            "hr_summary_diagnostics": [{key: value for key, value in summary.items() if key != "cells"} for summary in source["hr"]],
            "profiles": profiles}


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Build Hongxing catalog v2 JSON from the source workbook")
    parser.add_argument("workbook")
    parser.add_argument("--output", help="Write catalog JSON to this path instead of stdout")
    args = parser.parse_args()
    result = json.dumps(build(args.workbook), ensure_ascii=False, indent=2)
    if args.output:
        Path(args.output).write_text(result + "\n", encoding="utf-8")
    else:
        print(result)
