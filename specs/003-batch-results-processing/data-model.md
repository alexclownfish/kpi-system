# Data Model

## FinalScoreImportBatch

- `id`: 随机字符串主键
- `created_by`, `file_name`
- `status`: `previewed | committed | expired`
- `payload_json`, `summary_json`
- `expires_at`, `committed_at`, `created_at`, `updated_at`

提交只读取数据库内的预检结果，并重新执行状态、范围和并发校验。
