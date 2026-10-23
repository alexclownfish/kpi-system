# Feature Specification: Evaluation Workflow Unification

**Feature Branch**: `007-evaluation-workflow-unification`

**Created**: 2026-09-30

**Status**: Draft

**Input**: User description: "统一绩效考核评分任务、流程状态、模板快照、加权计算、角色职责与导出链路，先提供只读数据审计，再安全迁移进行中考核并保证历史结果稳定。按现有项目的 Spec Kit 流程完成需求、设计、任务拆分和一致性分析。"

## Clarifications

### Session 2026-10-01

- Q: 原表部分负责人模板维度内权重不等于 100%，如何处理？ → A: 采用 A，保留原表数值，以逐项四舍五入后的综合权重合计 100% 为保存、发布和计算校验标准；不要求维度内指标权重合计 100%，不归一化。

## User Scenarios & Testing *(mandatory)*

<!--
  IMPORTANT: User stories should be PRIORITIZED as user journeys ordered by importance.
  Each user story/journey must be INDEPENDENTLY TESTABLE - meaning if you implement just ONE of them,
  you should still have a viable MVP (Minimum Viable Product) that delivers value.

  Assign priorities (P1, P2, P3, etc.) to each story, where P1 is the most critical.
  Think of each story as a standalone slice of functionality that can be:
  - Developed independently
  - Tested independently
  - Deployed independently
  - Demonstrated to users independently
-->

### User Story 1 - Audit Inconsistent Assessments (Priority: P1)

As an HR administrator, I want to run a read-only audit of existing templates, assessments, scores, invitations, snapshots, and totals, so that I can understand which records are consistent, automatically repairable, or require manual review before any migration changes data.

**Why this priority**: Production data is currently inconsistent. A safe audit is the prerequisite for every later change and must not risk existing records.

**Independent Test**: Run the audit against a database containing valid, incomplete, duplicate, and legacy assessments; verify that it produces deterministic findings without changing row counts, scores, statuses, or timestamps.

**Acceptance Scenarios**:

1. **Given** an assessment with a missing template snapshot, **When** HR runs the audit, **Then** the assessment is reported as requiring manual review and no data is changed.
2. **Given** an assessment whose item count, assignment count, and score count are internally consistent, **When** HR runs the audit, **Then** it is reported as eligible for the next migration stage.
3. **Given** the same database and audit filters, **When** the audit is run twice, **Then** the report contains the same findings and does not create records.

---

### User Story 2 - Assign Explicit Scoring Tasks (Priority: P1)

As a participant in an assessment, I want each required item to have an explicit scoring task assigned to a specific user and business responsibility, so that I only see the items I am responsible for and every required item can be tracked to completion.

**Why this priority**: Explicit task ownership is the foundation for correct permissions, status transitions, multi-scorer aggregation, and dynamic UI rendering.

**Independent Test**: Create a template where item A is assigned to a project manager, item B to a department manager, and item C to both; create an assessment and verify that exactly those user-item assignments are created and enforced.

**Acceptance Scenarios**:

1. **Given** a published template with item-level scorer responsibilities, **When** an assessment is created, **Then** one task exists for each resolved user-item pair and duplicate tasks are prevented.
2. **Given** a scorer assigned only to item A, **When** that scorer opens the assessment, **Then** item B is not editable or submittable by that scorer.
3. **Given** a user changes their RBAC role after an assessment is created, **When** the assessment is viewed, **Then** the existing task assignment remains bound to the assessment snapshot.

---

### User Story 3 - Complete a Template-Driven Workflow (Priority: P1)

As an employee, scorer, HR reviewer, or administrator, I want the assessment to move through a single backend-controlled workflow based on required task completion, so that an assessment cannot reach employee confirmation while required self-evaluation or assigned scoring work is incomplete.

**Why this priority**: The current fixed self/manager/HR flow is the main source of process mismatch. The workflow must follow the template rather than hard-coded scorer names.

**Independent Test**: Create an assessment with required self-evaluation and two required scorer tasks; submit them in different orders and verify that the backend allows confirmation only after all required work is validly submitted.

**Acceptance Scenarios**:

1. **Given** one required scorer task is incomplete, **When** a user attempts to enter employee confirmation, **Then** the request is rejected with a structured explanation listing the incomplete task.
2. **Given** a valid score of `0`, **When** the scorer submits it, **Then** the task is treated as completed rather than missing.
3. **Given** all required tasks are complete, **When** the backend advances the assessment, **Then** it calculates the result and moves the assessment to the confirmation state without accepting a client-supplied total score.

---

### User Story 4 - Calculate One Authoritative Result (Priority: P1)

As an HR administrator, I want every assessment result to be calculated from its immutable template snapshot using one calculation version, so that the detail page, review flow, statistics, Excel export, and PDF export all show the same weighted result.

**Why this priority**: The same assessment currently has multiple possible totals depending on the page or endpoint used. A single authoritative result prevents further data drift.

**Independent Test**: Use a template with dimension and item weights plus multiple scorer assignments; submit known scores and compare the backend result across detail, export, and statistics responses.

**Acceptance Scenarios**:

1. **Given** dimension and item weights, **When** the assessment is calculated, **Then** the total equals the sum of item final scores multiplied by their combined weights.
2. **Given** multiple valid scores for one item, **When** the template uses the default aggregation rule, **Then** the item final score is the average of valid scores and `0` remains a valid score.
3. **Given** a client submits a forged `total_score`, **When** the assessment is saved or advanced, **Then** the backend ignores the forged value and persists the server-calculated result.
4. **Given** original department-head templates for structure, electronics and application software with non-100 internal item totals but composite totals of 100%, **When** HR saves, publishes and completes scoring, **Then** original weights remain unchanged and the deterministic source fixtures yield 24.82/D, 51.07/D and 33.71/D respectively.

### User Story 5 - Preserve Historical Assessments and Unify Exports (Priority: P2)

As an HR administrator, I want historical assessments and all export formats to use the assessment-time snapshot and one result representation, so that template edits, role changes, and export layout changes do not rewrite history or produce contradictory files.

**Why this priority**: Historical evidence and signed documents must remain stable for employee confirmation and audit purposes.

**Independent Test**: Create and calculate an assessment, modify the current template, then compare the original detail, Excel, detail PDF, and batch sign-off PDF outputs; all must retain the original fields, labels, scores, orientation, and totals.

**Acceptance Scenarios**:

1. **Given** a completed assessment with a template snapshot, **When** the current template is renamed or its weights are changed, **Then** the historical assessment result and export content remain unchanged.
2. **Given** an export layout selected in the template, **When** Excel and PDF exports are generated, **Then** both contain the same selected fields and the configured print orientation.
3. **Given** an export or calculation error, **When** the request fails, **Then** the page displays an actionable error and no partial download is presented as successful.

### User Story 6 - Separate RBAC From Evaluation Responsibilities (Priority: P2)

As a system administrator, I want system roles and evaluation responsibilities to be managed independently, so that a user may have ordinary application permissions while still being assigned a project-manager, product-manager, department-manager, or CTO scoring responsibility for a defined scope.

**Why this priority**: Treating business scorer types as RBAC roles currently causes role changes to break existing scoring assignments and prevents reliable matching.

**Independent Test**: Assign a user a normal RBAC role and a project-manager responsibility, create an applicable assessment, then remove the responsibility for future assessments while verifying existing assessment assignments remain unchanged.

**Acceptance Scenarios**:

1. **Given** a user with a valid evaluation responsibility but no elevated RBAC role, **When** an applicable assessment is created, **Then** the user receives only the scorer tasks granted by the template and scope.
2. **Given** a responsibility is removed, **When** a new assessment is created, **Then** the user is not selected for new tasks, while existing assessment snapshots remain intact.
3. **Given** a user without an assigned task, **When** they call a scoring endpoint, **Then** the backend returns a structured forbidden response.

### User Story 7 - Migrate Safely and Converge (Priority: P3)

As a product owner, I want legacy assessments to be migrated in controlled batches with a dry-run, explicit review markers, and rollback-safe records, so that the new workflow can be adopted without silently rewriting ambiguous production history.

**Why this priority**: Existing data contains legacy fields and inconsistent assignments. Controlled migration is required before the old model can be retired.

**Independent Test**: Run a dry-run migration on a fixture containing clean and ambiguous assessments; verify only clean records are proposed for automatic migration and ambiguous records are marked for manual review with an explanation.

**Acceptance Scenarios**:

1. **Given** a clean in-progress assessment with a complete snapshot, **When** the migration dry-run runs, **Then** it proposes deterministic scoring tasks and reports no destructive actions.
2. **Given** an ambiguous assessment with conflicting legacy scores, **When** the migration dry-run runs, **Then** it leaves the assessment unchanged and records a manual-review reason.
3. **Given** a migration batch is applied, **When** the batch completes, **Then** an audit record identifies the operator, source version, target version, affected assessments, and any skipped records.

### User Story 8 - Supplement Completed Sign-off Evidence (Priority: P2)

As an HR operator, I want to append a scanned paper-signature image after an employee has already confirmed an assessment online, so that the physical document can be archived without reopening the assessment or changing its finalized result.

**Why this priority**: Online confirmation and later paper collection are both valid operational steps. The paper copy is evidence attached to an already finalized result and must not create a second confirmation or reopen scoring.

**Independent Test**: Complete an assessment online, upload a valid paper-signature file as an authorized operator, and verify that the assessment remains `completed`, the original confirmation and result snapshot are byte-for-byte unchanged, the supplement is listed/downloadable by an in-scope reader, and an audit entry identifies the upload.

**Acceptance Scenarios**:

1. **Given** a `completed` assessment with an existing final confirmation, **When** an authorized in-scope operator uploads a PDF, JPG, JPEG, or PNG no larger than 10 MB, **Then** one append-only supplement is stored and the response reports `completed` without changing the canonical result, total, snapshot checksum/version, or original confirmation.
2. **Given** a completed assessment, **When** the operator uploads more than one paper-signature file, **Then** each upload receives its own ID, uploader, upload time, optional signed date, original filename, content type, size, and remark; earlier supplements remain available and are never overwritten.
3. **Given** an assessment that is awaiting confirmation, rejected, cancelled, or missing its final confirmation, **When** a user attempts to upload a supplement, **Then** the request is rejected and no database row, file, status transition, or audit success entry is created.
4. **Given** an invalid, empty, unsupported, or oversized file, **When** an upload is attempted, **Then** the request is rejected before it is exposed as a supplement and no orphan file or partial record remains.
5. **Given** a user with assessment view access in the assessment's data scope, **When** they list or download supplements, **Then** they can access only that assessment's materials; an out-of-scope user cannot list or download them, and reads do not create audit or business records.
6. **Given** a successful supplement upload, **When** an administrator inspects the audit log, **Then** exactly one `upload_signoff_supplement` success entry links the evaluation and supplement while excluding the free-text remark and other sensitive contents.
7. **Given** an assessment with supplements, **When** the assessment is deleted through the normal transactional path, **Then** supplement rows and their stored files are removed together and no orphan sign-off files remain.

### Edge Cases

<!--
  ACTION REQUIRED: The content in this section represents placeholders.
  Fill them out with the right edge cases.
-->

- A required scorer cannot be resolved from the responsibility scope: the assessment creation is blocked with a remediation message, unless HR explicitly selects a replacement scorer.
- Two responsibility rules resolve the same user-item pair: one task is created, the source rules are retained in the snapshot, and the conflict is reported for review.
- A score of `0` is submitted: it is valid and must not be confused with missing input.
- An item is marked not applicable: it must be represented distinctly from an unsubmitted score and follow the template's required-item policy.
- A scorer submits while another tab has already submitted a newer version: the older request is rejected without overwriting the newer score.
- A published template is edited through a legacy endpoint: the write is rejected and the user is directed to create a new template version.
- An export is requested for a legacy assessment without a complete snapshot: the system uses the preserved legacy result, labels the calculation version, and warns that migration is required instead of silently recalculating.
- A GET request loads an assessment: it must not create invitations, scoring tasks, or audit records as a side effect.
- An employee confirms online and the paper signature arrives later: the paper file is an append-only supplement to the completed record; it must not replace the online confirmation, create a second final confirmation, or reopen scoring.
- A completed assessment has an existing paper confirmation: any later paper copy is still recorded as a supplement and does not overwrite the original attachment or confirmation metadata.
- A supplement upload succeeds in storage but its database insert fails: the stored file is removed and no success audit entry is written.
- Two users upload supplements concurrently: both valid files may be retained with distinct IDs; neither request may mutate the assessment status or the other supplement.
- A supplement's original filename contains path separators or unsafe characters: only a safe generated storage key is used for the file path, while the basename is retained as display metadata.
- A template has an internal item-weight total other than 100%: this alone is not an error; only the rounded composite total is required to equal 100%. Non-finite/out-of-range weights, empty dimensions, and inconsistent snapshot composite values are rejected without rewriting history.

## Requirements *(mandatory)*

<!--
  ACTION REQUIRED: The content in this section represents placeholders.
  Fill them out with the right functional requirements.
-->

### Functional Requirements

- **FR-001**: The system MUST provide a read-only audit that reports template, snapshot, item, score, task, invitation, status, duplicate, and total-calculation consistency for selected assessments.
- **FR-002**: The audit MUST be deterministic and MUST NOT create, update, delete, or reorder business records.
- **FR-003**: The system MUST create one explicit scoring task for each unique assessment-item-scorer-user pair.
- **FR-004**: Each scoring task MUST record the resolved business responsibility, required flag, status, score state, submission time, and optimistic-concurrency version.
- **FR-005**: The system MUST derive actual scorer users from template responsibility rules and scoped assignments, while preserving the resolved assignment in the assessment snapshot.
- **FR-006**: The system MUST keep RBAC roles and evaluation responsibilities as separate concepts and MUST NOT require a scorer to hold an identically named RBAC role.
- **FR-007**: The backend MUST authorize score writes using both system permission and the current user's explicit task assignment; frontend visibility MUST NOT be the security boundary.
- **FR-008**: The workflow MUST use backend-controlled states that represent self-evaluation, scoring, review, employee confirmation, completion, rejection, and cancellation without assuming a fixed manager/HR sequence.
- **FR-009**: The workflow MUST block confirmation until every required self-evaluation and required scoring task has a valid submission or an explicitly allowed not-applicable state.
- **FR-010**: The system MUST treat `0` as a valid score and MUST distinguish it from missing, rejected, and not-applicable states.
- **FR-011**: The backend MUST calculate item results and total results from the assessment's immutable template snapshot and calculation version.
- **FR-012**: The backend MUST apply the configured dimension weight and item weight rules, aggregate multiple scorer values according to the snapshot rule, and MUST NOT trust a client-supplied total score.
- **FR-013**: The system MUST persist a canonical calculation result that can be reused by detail views, confirmation, exports, and statistics.
- **FR-014**: Historical assessments MUST read labels, weights, scorer rules, and export layout from their assessment-time snapshot rather than the current template.
- **FR-015**: Detail export, Excel export, PDF export, and batch sign-off PDF MUST use the same canonical result and MUST honor the same selected fields and print orientation.
- **FR-016**: All write operations MUST return stable success or error structures and the UI MUST show submitting, success, and actionable failure states.
- **FR-017**: Published templates MUST be immutable; changes MUST create a new version and MUST NOT alter existing assessment snapshots.
- **FR-018**: GET/read endpoints MUST NOT create scoring tasks, invitations, or other business records as side effects.
- **FR-019**: Migration MUST support dry-run, deterministic eligibility, manual-review markers, operator/audit records, and preservation of legacy results.
- **FR-020**: Migration MUST NOT delete existing assessments, scores, invitations, or persisted volumes as part of normal deployment.
- **FR-021**: Every new or changed protected API MUST document request, response, validation, authorization, and error behavior in the feature contracts.
- **FR-022**: The implementation MUST include automated regression coverage for allowed and denied scoring, data-scope boundaries, zero scores, status transitions, snapshot immutability, calculation consistency, export consistency, and migration dry-run behavior.
- **FR-023**: Save, publish and canonical calculation MUST preserve original weights, compute each composite weight with decimal HALF_UP rounding to one decimal, and require their sum to equal 100%. Internal item totals and dimension totals are informational, not additional total constraints. Individual dimension/item weights MUST be finite numbers in 0..100 with nonempty matching groups. Stored snapshot composite values MUST agree with the formula. No normalization or historical data rewrite is permitted.
- **FR-024**: For an assessment in `completed` state with an existing final confirmation, the system MUST allow an authorized in-scope operator to append paper-signature evidence without changing the assessment status, canonical result, total, result snapshot, snapshot checksum/version, or original confirmation row.
- **FR-025**: Supplement uploads MUST require a non-empty PDF, JPG, JPEG, or PNG file of at most 10 MB; an optional signed date MUST use `YYYY-MM-DD`, and the optional remark MUST be limited to 500 characters. Invalid uploads MUST leave no partial database row or stored file.
- **FR-026**: Each supplement MUST be an append-only record containing the assessment, uploader, upload time, optional signed date, safe stored filename, original basename, detected content type, byte size, and remark. Multiple supplements are allowed; the original confirmation attachment and metadata MUST remain immutable.
- **FR-027**: Supplement write access MUST require the existing assessment approval permission and data-scope access. Listing and downloading MUST require assessment view access and the same data-scope boundary. Supplement read endpoints MUST have no business-write side effects.
- **FR-028**: Every successful supplement upload MUST create an auditable `upload_signoff_supplement` operation containing the evaluation and supplement identifiers plus non-sensitive file metadata. Free-text remarks and raw file contents MUST NOT be copied to the audit details.
- **FR-029**: Normal transactional assessment deletion MUST remove all supplement rows and their stored files, with rollback/cleanup behavior that prevents orphan sign-off attachments.

### Key Entities *(include if feature involves data)*

- **Assessment Audit Finding**: A deterministic read-only observation about a template, assessment, task, score, snapshot, status, duplicate, or calculation inconsistency, including severity and suggested remediation.
- **Evaluation Scoring Task**: The assignment of one concrete user to score one item in one assessment, including responsibility type, required state, score state, submission metadata, and version.
- **Evaluation Responsibility**: A business responsibility such as project manager, department manager, product manager, or CTO, scoped to company, department, project, or job position and independent from RBAC roles.
- **Assessment Snapshot**: The immutable assessment-time copy of template metadata, dimensions, items, weights, required rules, scorer rules, resolved users, export layout, and calculation version.
- **Canonical Calculation Result**: The server-owned result containing item-level scores, aggregation details, weighted contributions, total score, grade, and calculation version.
- **Migration Batch**: A controlled dry-run or applied set of legacy assessments with eligibility, manual-review reasons, operator identity, source version, target version, and audit trail.
- **Evaluation Sign-off Supplement**: An append-only paper-signature attachment added after final confirmation, with uploader, timestamps, optional signed date, file metadata, remark, and audit linkage; it never becomes a second confirmation.

## Success Criteria *(mandatory)*

<!--
  ACTION REQUIRED: Define measurable success criteria.
  These must be technology-agnostic and measurable.
-->

### Measurable Outcomes

- **SC-001**: For a fixed database snapshot, repeated audits produce identical findings and change zero business rows.
- **SC-002**: For 100% of newly created assessments, every required item has an explicit resolved scoring task or a blocking, actionable creation error.
- **SC-003**: For 100% of newly created assessments in acceptance tests, detail, Excel, PDF, batch sign-off PDF, and statistics expose the same total score and calculation version.
- **SC-004**: In acceptance tests, no user without an explicit task can write a score, even when the user has a general review permission.
- **SC-005**: In acceptance tests, a required score of `0` allows progression exactly as a positive score does, while an unsubmitted score blocks progression.
- **SC-006**: After a published template is edited or a user's role/responsibility changes, 100% of previously created assessment snapshots and completed exports remain unchanged.
- **SC-007**: A dry-run migration reports all eligible and ambiguous records without modifying existing scores, statuses, invitations, or snapshots.
- **SC-008**: Every protected write failure is shown in the UI with a human-readable reason and no silent failure path remains for the assessment workflow.
- **SC-009**: In acceptance tests, 100% of valid post-completion supplement uploads leave the status, total, canonical result, snapshot checksum/version, and original confirmation unchanged.
- **SC-010**: In acceptance tests, every successful supplement upload is listed and downloadable by an in-scope reader, while out-of-scope reads and all non-completed/missing-confirmation uploads are denied without a new row or file.
- **SC-011**: For every successful supplement upload, exactly one audit entry links the evaluation and supplement and contains no free-text remark; failed uploads leave no stored-file or database residue.
- **SC-012**: Deleting a fixture assessment with supplements leaves zero related supplement rows and zero stored supplement files after the transaction completes.

## Assumptions

<!--
  ACTION REQUIRED: The content in this section represents placeholders.
  Fill them out with the right assumptions based on reasonable defaults
  chosen when the feature description did not specify certain details.
-->

- Existing JWT authentication, permission, audit, export, and Docker deployment infrastructure will be reused where it can meet the new contracts.
- The default aggregation rule for multiple scorer values is arithmetic average; future templates may add an explicit scorer-weight rule without changing the task model.
- HR review is optional and only exists when the template or workflow configuration requires it; it is not assumed for every assessment.
- A score of `0` is valid; `null` means not submitted; not-applicable is a distinct explicit state.
- Published templates are versioned and immutable; draft editing remains available.
- Mobile-specific layout, external SSO, and deletion of legacy assessment history are out of scope for this feature.
- Production migration is staged: audit and dry-run first, then explicit application by an authorized operator.
- Existing legacy assessments remain readable with a legacy calculation version even if they cannot be safely migrated automatically.
