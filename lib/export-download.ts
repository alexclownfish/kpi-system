export interface ExportDownloadResponse {
  file_url?: string
  request_id?: string
}

export interface ExportDownloadOptions {
  downloadUrl: (url: string) => Promise<unknown>
  openWindow: (url: string, target: string) => unknown
}

/**
 * Raised when the export response was received but the file itself could not
 * be handed to the browser. The request id is retained so the UI can include
 * it in the diagnostic message shown to the user.
 */
export class ExportDownloadError extends Error {
  readonly requestId?: string

  constructor(message: string, requestId?: string, options?: { cause?: unknown }) {
    super(message, options)
    this.name = "ExportDownloadError"
    this.requestId = requestId
  }
}

/**
 * Download an export and use a new browser window only when the embedded
 * download bridge rejects. A null return from window.open means the browser
 * blocked the fallback, so callers receive an error and must not report
 * success.
 */
export async function downloadExportFile(
  response: ExportDownloadResponse,
  options: ExportDownloadOptions,
): Promise<void> {
  const fileUrl = response.file_url?.trim()
  if (!fileUrl) {
    throw new ExportDownloadError("导出文件地址缺失，请重新生成导出文件。", response.request_id)
  }

  try {
    await options.downloadUrl(fileUrl)
    return
  } catch (downloadError) {
    try {
      const fallbackWindow = options.openWindow(fileUrl, "_blank")
      if (fallbackWindow) return
    } catch (fallbackError) {
      throw new ExportDownloadError(
        "导出文件下载失败，备用窗口打开失败，请检查浏览器设置后重试。",
        response.request_id,
        { cause: fallbackError },
      )
    }

    throw new ExportDownloadError(
      "导出文件下载失败，浏览器阻止了备用窗口，请允许弹窗后重试。",
      response.request_id,
      { cause: downloadError },
    )
  }
}

export function getExportDownloadRequestId(error: unknown): string | undefined {
  return error instanceof ExportDownloadError ? error.requestId : undefined
}
