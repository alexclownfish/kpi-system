import type { APIRequestContext, APIResponse } from "@playwright/test"
import { redact, safeError } from "./redaction"

export interface LoginResult {
  token: string
  user: { id: number; email: string; role: string; department_id: number }
  permissions: string[]
  data_scope: string
}

export class AcceptanceApiError extends Error {
  constructor(
    message: string,
    readonly status?: number,
    readonly body?: unknown,
  ) {
    super(message)
  }
}

export class AcceptanceApiClient {
  constructor(
    private readonly request: APIRequestContext,
    readonly baseUrl: string,
  ) {}

  async health(): Promise<void> {
    const response = await this.request.get(`${this.baseUrl}/health`)
    const body = await this.readBody(response)
    if (response.status() !== 200 || (body as { status?: string })?.status !== "OK") {
      throw new AcceptanceApiError("Health precondition failed", response.status(), redact(body))
    }
  }

  async login(email: string, password: string): Promise<LoginResult> {
    const response = await this.request.post(`${this.baseUrl}/api/auth/login`, { data: { email, password } })
    const body = await this.readBody(response)
    if (response.status() !== 200) {
      throw new AcceptanceApiError("HR authentication failed", response.status(), redact(body))
    }
    return body as LoginResult
  }

  async authorized(
    token: string,
    method: "GET" | "POST" | "PUT" | "DELETE",
    path: string,
    data?: unknown,
  ): Promise<{ response: APIResponse; body: unknown }> {
    try {
      const response = await this.request.fetch(`${this.baseUrl}${path}`, {
        method,
        data,
        headers: { Authorization: `Bearer ${token}` },
      })
      const body = await this.readBody(response)
      if (response.status() >= 500) {
        throw new AcceptanceApiError("Service error is not an authorization result", response.status(), redact(body))
      }
      return { response, body }
    } catch (error) {
      if (error instanceof AcceptanceApiError) throw error
      throw new AcceptanceApiError(`Network request failed: ${safeError(error)}`)
    }
  }

  private async readBody(response: APIResponse): Promise<unknown> {
    const text = await response.text()
    if (!text) return null
    try {
      return JSON.parse(text)
    } catch {
      return redact(text)
    }
  }
}
