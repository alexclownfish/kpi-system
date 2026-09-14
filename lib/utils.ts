import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"
import { KPIEvaluation, RecentEvaluation } from "./api"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

// 获取考核周期标签
export function getPeriodLabel(period: string) {
  switch (period) {
    case "monthly":
      return "月度"
    case "quarterly":
      return "季度"
    default:
      return "年度"
  }
}

// 获取考核周期值
export function getPeriodValue(evaluation: KPIEvaluation | RecentEvaluation) {
  const values = [`${evaluation.year}`]
  if (evaluation.month) values.push(`年${evaluation.month}月`)
  if (evaluation.quarter) values.push(`年Q${evaluation.quarter}`)
  return getPeriodLabel(evaluation.period) + " " + values.join("")
}

// 判断是否未知
export function isUnknown(value: null | undefined | unknown) {
  return value === null || value === undefined
}

// 格式化浮点数到1位小数，去掉不必要的尾随零
export function formatScore(score: number | null | undefined): string {
  if (score === null || score === undefined || isNaN(score)) {
    return "0"
  }
  // 保留1位小数并去掉尾随零
  return parseFloat(score.toFixed(1)).toString()
}

// 格式化浮点数到1位小数，返回数字类型
export function formatScoreNumber(score: number | null | undefined): number {
  if (score === null || score === undefined || isNaN(score)) {
    return 0
  }
  return parseFloat(score.toFixed(1))
}

// 导出一个函数，用于生成输入框的占位符
export function generateInputPlaceholder(maxScore: number) {
  if (maxScore < 0) {
    // 如果最大分数小于等于0，输入应在maxScore-0之间
    return `${maxScore}-0`
  } else if (maxScore > 0) {
    // 正常情况，输入应在0-maxScore之间
    return `0-${maxScore}`
  }
  // 任意分数
  return "请输入分数"
}

// 评分输入验证
export function scoreInputValidation(e: React.FormEvent<HTMLInputElement>, maxScore: number) {
  const input = e.target as HTMLInputElement
  const value = parseFloat(input.value)
  if (isNaN(value)) {
    return
  }
  if (maxScore < 0) {
    // 如果最大分数小于等于0，输入应在maxScore-0之间
    if (value > 0) {
      input.value = "0"
    } else if (value < maxScore) {
      input.value = maxScore.toString()
    }
  } else if (maxScore > 0) {
    // 正常情况，输入应在0-maxScore之间
    if (value > maxScore) {
      input.value = maxScore.toString()
    } else if (value < 0) {
      input.value = "0"
    }
  }
  // 如果分数小数点超过1位，则截取前1位
  if (/\.\d{2,}/.test(input.value)) {
    input.value = input.value.slice(0, input.value.indexOf('.') + 2)
  }
}

// 查找父级滚动容器
export function findScrollContainer(element: HTMLElement) {
  let parent = element.parentElement
  while (parent) {
    const style = window.getComputedStyle(parent)
    if (style.overflow === 'auto' || style.overflow === 'scroll' || style.overflowY === 'auto' || style.overflowY === 'scroll') {
      return parent
    }
    parent = parent.parentElement
  }
}

// 滚动到指定元素
export function scrollToElement(element: HTMLElement, offset: number = 0) {
  const scrollContainer = findScrollContainer(element)
  if (scrollContainer) {
    const containerRect = scrollContainer.getBoundingClientRect()
    const elementRect = element.getBoundingClientRect()
    const targetScrollTop = scrollContainer.scrollTop + elementRect.top - containerRect.top - offset
    scrollContainer.scrollTo({ top: targetScrollTop, behavior: "smooth" })
  } else {
    window.scrollTo({ top: element.offsetTop - offset, behavior: "smooth" })
  }
}