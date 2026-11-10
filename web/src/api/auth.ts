import {request} from './http'
import type {
    LoginResponse,
    RefreshTokenResponse,
    UserSummary,
    WsTicketResponse,
} from '@/types/api'

interface RegisterInput {
    username: string
    password: string
    confirm_password: string
    nickname: string
}

interface UpdateProfileInput {
    nickname: string
    bio: string
    avatar_url?: string
}

// register 提交注册表单并在成功后自动登录
export function register(input: RegisterInput): Promise<UserSummary> {
    return request('/auth/register', {
        method: 'POST',
        body: JSON.stringify(input),
    })
}

// login 调用登录接口并返回令牌、设备与用户摘要
export function login(
    username: string,
    password: string,
    deviceID: string,
): Promise<LoginResponse> {
    return request('/auth/login', {
        method: 'POST',
        body: JSON.stringify({
            username,
            password,
            device_id: deviceID || undefined,
            device_name: 'RelayIM Web',
            platform: 1,
        }),
    })
}

// refreshToken 使用 Refresh Token 换取新的访问令牌
export function refreshToken(refreshToken: string): Promise<RefreshTokenResponse> {
    return request('/auth/refresh', {
        method: 'POST',
        body: JSON.stringify({refresh_token: refreshToken}),
    })
}

// logout 撤销当前访问令牌和刷新令牌
export function logout(token: string, refreshToken: string): Promise<void> {
    return request(
        '/auth/logout',
        {
            method: 'POST',
            body: JSON.stringify({refresh_token: refreshToken || undefined}),
        },
        token,
    )
}

// getMe 读取当前登录用户资料
export function getMe(token: string): Promise<UserSummary> {
    return request('/me', {}, token)
}

// updateProfile 保存昵称、简介和头像地址
export function updateProfile(
    token: string,
    input: UpdateProfileInput,
): Promise<UserSummary> {
    return request(
        '/me',
        {
            method: 'PUT',
            body: JSON.stringify(input),
        },
        token,
    )
}

// uploadAvatar 以 multipart 方式上传头像文件
export function uploadAvatar(token: string, file: File): Promise<UserSummary> {
    return new Promise((resolve, reject) => {
        const xhr = new XMLHttpRequest()
        xhr.open('POST', '/api/v1/me/avatar')
        xhr.setRequestHeader('Authorization', `Bearer ${token}`)
        xhr.onerror = () => reject(new Error('上传头像失败'))
        xhr.onload = () => {
            let body: unknown
            try {
                body = JSON.parse(xhr.responseText)
            } catch {
                body = null
            }
            if (xhr.status < 200 || xhr.status >= 300) {
                const message =
                    body &&
                    typeof body === 'object' &&
                    'error' in body &&
                    typeof body.error === 'string'
                        ? body.error
                        : `上传头像失败（${xhr.status}）`
                reject(new Error(message))
                return
            }
            resolve(body as UserSummary)
        }
        const data = new FormData()
        data.append('file', file)
        xhr.send(data)
    })
}

// removeAvatar 移除当前用户头像并返回最新资料
export function removeAvatar(token: string): Promise<UserSummary> {
    return request(
        '/me/avatar',
        {method: 'DELETE'},
        token,
    )
}

// issueWsTicket 申请一次性 WebSocket Ticket 并绑定当前设备
export function issueWsTicket(
    token: string,
    deviceId: string,
): Promise<WsTicketResponse> {
    return request(
        '/auth/ws-ticket',
        {
            method: 'POST',
            body: JSON.stringify({
                device_id: deviceId || undefined,
                device_name: 'RelayIM Web',
                platform: 1,
            }),
        },
        token,
    )
}
