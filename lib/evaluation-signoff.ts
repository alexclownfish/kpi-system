const paperSignoffStates = new Set(["pending_confirm", "awaiting_employee_confirmation"])

/**
 * Paper sign-off is the administrator/HR completion path for an assessment
 * whose final result is ready for employee confirmation. A completed record
 * already has confirmation evidence and must not expose a second upload.
 */
export function canRegisterPaperSignoff(
  status: string,
  canApprove: boolean,
  hasObjection: boolean,
): boolean {
  return canApprove && !hasObjection && paperSignoffStates.has(status)
}
