package utils

import (
	"fmt"
	"strings"
)

// 获取考核周期标签
func GetPeriodLabel(period string) string {
	switch period {
	case "monthly":
		return "月度"
	case "quarterly":
		return "季度"
	default:
		return "年度"
	}
}

// 获取考核周期值
// period: "monthly" / "quarterly" / 其他
// year: 年份
// month: 月份（可选，0表示无）
// quarter: 季度（可选，0表示无）
func GetPeriodValue(period string, year int, month *int, quarter *int) string {
	values := []string{fmt.Sprintf("%d", year)}
	if month != nil && *month > 0 {
		values = append(values, fmt.Sprintf("年%d月", *month))
	}
	if quarter != nil && *quarter > 0 {
		values = append(values, fmt.Sprintf("年Q%d", *quarter))
	}
	return GetPeriodLabel(period) + " " + strings.Join(values, "")
}
