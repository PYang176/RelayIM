import type {ApiErrorBody} from '@/types/api'

const API_BASE = '/api/v1'

export class ApiError extends Error {
    readonly status: number

    constructor(message: string, status: number) {
        super(message)
        this.name = 'ApiError'
        this.status = status
    }
}

// request 发送带认证信息的 API 请求并统一映射错误
export async function request<T>(
    path: string,
    options: RequestInit = {},
    token?: string,
): Promise<T> {
    const headers = new Headers(options.headers)
    if (options.body && !headers.has('Content-Type')) {
        headers.set('Content-Type', 'application/json')
    }
    if (token) {
        headers.set('Authorization', `Bearer ${token}`)
    }

    const response = await fetch(`${API_BASE}${path}`, {
        ...options,
        headers,
    })
    if (!response.ok) {
        let message = `请求失败（${response.status}）`
        try {
            const body = (await response.json()) as ApiErrorBody
            if (body.error) {
                message = body.error
            }
        } catch {
            // 非 JSON 错误响应保留通用状态码信息。
        }
        throw new ApiError(message, response.status)
    }
    if (response.status === 204) {
        return undefined as T
    }
    return (await response.json()) as T
}
