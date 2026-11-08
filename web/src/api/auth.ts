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

export function register(input: RegisterInput): Promise<UserSummary> {
    return request('/auth/register', {
        method: 'POST',
        body: JSON.stringify(input),
    })
}

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
            device_name: 'GinChat Web',
            platform: 1,
        }),
    })
}

export function refreshToken(refreshToken: string): Promise<RefreshTokenResponse> {
    return request('/auth/refresh', {
        method: 'POST',
        body: JSON.stringify({refresh_token: refreshToken}),
    })
}

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

export function getMe(token: string): Promise<UserSummary> {
    return request('/me', {}, token)
}

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
                device_name: 'GinChat Web',
                platform: 1,
            }),
        },
        token,
    )
}
