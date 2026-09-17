# API Contracts

- `GET /api/final-scores/import-template?...`
- `POST /api/final-scores/import/preview` (`multipart/form-data`, `file`)
- `POST /api/final-scores/import/:batchId/commit` (`{valid_only:true}`)
- `GET /api/export/signoff-batch?...`
- `GET /api/statistics/signoffs?...`

筛选参数：`department_id`, `year`, `period`, `month`, `quarter`, `status`。
