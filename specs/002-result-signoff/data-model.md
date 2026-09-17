# Data Model: 绩效结果打印签字与归档

## EvaluationResultSnapshot

一条考核正式结果的不可变版本。

- `id`: 主键
- `evaluation_id`: 必填，关联考核并建立索引
- `version`: 正整数；同一考核内唯一且递增
- `snapshot_json`: 必填，规范化结果 JSON
- `checksum`: 64 位 SHA-256 十六进制；同一考核内唯一
- `created_by`: 创建该版本的用户
- `created_at`: 创建时间

关系：一个考核可有多个版本；一个版本最多被一个有效确认记录引用。

## EvaluationConfirmation

考核最终确认凭证。

- `id`: 主键
- `evaluation_id`: 必填且唯一，一条考核只允许一个有效确认
- `snapshot_id`: 必填，关联确认时的结果版本
- `method`: `online` 或 `paper`
- `confirmed_by`: 员工确认人；纸质方式为被考核员工
- `confirmed_at`: 系统登记确认时间
- `signed_at`: 纸质实际签字日期；在线方式为空
- `handled_by`: 纸质回录经办人；在线方式为空
- `attachment_stored_name`: 纸质方式必填，随机存储名
- `attachment_original_name`: 纸质方式必填，原文件名
- `attachment_content_type`: 纸质方式必填
- `attachment_size`: 纸质方式必填，1 到 10 MB
- `remark`: 可选，最多 500 字符
- `created_at`, `updated_at`: 审计时间

## 结果状态转换

```text
pending_confirm --online employee confirm--> completed + online confirmation
pending_confirm --paper HR record---------> completed + paper confirmation
completed --------------------------------> locked
```

纸质确认前必须存在与当前结果 checksum 一致的导出快照。在线确认可在事务内创建或复用当前快照。
