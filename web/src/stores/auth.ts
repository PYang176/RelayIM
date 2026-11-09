import {defineStore} from 'pinia'
import {computed, ref} from 'vue'

import * as authApi from '@/api/auth'
import type {UserSummary} from '@/types/api'

const TOKEN_KEY = 'ginchat.access_token'
const REFRESH_TOKEN_KEY = 'ginchat.refresh_token'
const DEVICE_KEY = 'ginchat.device_id'
const DEVICE_IDS_KEY = 'ginchat.device_ids'
const USER_KEY = 'ginchat.user'

function accountDeviceKey(username: string) {
    return username.trim().toLocaleLowerCase()
}

function readDeviceIDs(): Record<string, string> {
    const value = localStorage.getItem(DEVICE_IDS_KEY)
    if (!value) return {}
    try {
        const parsed = JSON.parse(value) as Record<string, unknown>
        return Object.fromEntries(
            Object.entries(parsed).filter(
                (entry): entry is [string, string] =>
                    typeof entry[1] === 'string',
            ),
        )
    } catch {
        localStorage.removeItem(DEVICE_IDS_KEY)
        return {}
    }
}

function readAccountDeviceID(username: string): string {
    return readDeviceIDs()[accountDeviceKey(username)] ?? ''
}

function readStoredUser(): UserSummary | null {
    const value = localStorage.getItem(USER_KEY)
    if (!value) return null
    try {
        return JSON.parse(value) as UserSummary
    } catch {
        localStorage.removeItem(USER_KEY)
        return null
    }
}

export const useAuthStore = defineStore('auth', () => {
    const token = ref(localStorage.getItem(TOKEN_KEY) ?? '')
    const refreshToken = ref(localStorage.getItem(REFRESH_TOKEN_KEY) ?? '')
    const deviceID = ref(localStorage.getItem(DEVICE_KEY) ?? '')
    const user = ref<UserSummary | null>(readStoredUser())

    const isAuthenticated = computed(() => Boolean(token.value))

    function persist(
        nextToken: string,
        nextRefreshToken: string,
        nextDeviceID: string,
        nextUser: UserSummary,
    ) {
        token.value = nextToken
        refreshToken.value = nextRefreshToken
        deviceID.value = nextDeviceID
        user.value = nextUser
        localStorage.setItem(TOKEN_KEY, nextToken)
        localStorage.setItem(REFRESH_TOKEN_KEY, nextRefreshToken)
        if (nextDeviceID) {
            localStorage.setItem(DEVICE_KEY, nextDeviceID)
            const deviceIDs = readDeviceIDs()
            deviceIDs[accountDeviceKey(nextUser.username)] = nextDeviceID
            localStorage.setItem(DEVICE_IDS_KEY, JSON.stringify(deviceIDs))
        }
        localStorage.setItem(USER_KEY, JSON.stringify(nextUser))
    }

    async function login(username: string, password: string) {
        /*
         * 同一浏览器可以依次登录多个账号。优先读取该账号自己的
         * 设备绑定，旧版单值缓存仅用于首次兼容迁移。
         */
        const knownDeviceID =
            readAccountDeviceID(username) || deviceID.value
        const response = await authApi.login(
            username,
            password,
            knownDeviceID,
        )
        persist(
            response.token,
            response.refresh_token,
            response.device_id,
            response.user,
        )
    }

    async function register(input: {
        username: string
        password: string
        confirmPassword: string
        nickname: string
    }) {
        await authApi.register({
            username: input.username,
            password: input.password,
            confirm_password: input.confirmPassword,
            nickname: input.nickname,
        })
        await login(input.username, input.password)
    }

    async function refreshSession() {
        if (!refreshToken.value) {
            throw new Error('Refresh Token 不存在')
        }
        const response = await authApi.refreshToken(refreshToken.value)
        token.value = response.token
        refreshToken.value = response.refresh_token
        localStorage.setItem(TOKEN_KEY, response.token)
        localStorage.setItem(REFRESH_TOKEN_KEY, response.refresh_token)
    }

    async function restoreUser() {
        if (!token.value) return
        try {
            user.value = await authApi.getMe(token.value)
        } catch (cause) {
            if (!refreshToken.value) {
                throw cause
            }
            await refreshSession()
            user.value = await authApi.getMe(token.value)
        }
        localStorage.setItem(USER_KEY, JSON.stringify(user.value))
    }

    async function updateProfile(input: {
        nickname: string
        bio: string
    }) {
        user.value = await authApi.updateProfile(token.value, {
            nickname: input.nickname,
            bio: input.bio,
        })
        localStorage.setItem(USER_KEY, JSON.stringify(user.value))
    }

    async function uploadAvatar(file: File) {
        user.value = await authApi.uploadAvatar(token.value, file)
        localStorage.setItem(USER_KEY, JSON.stringify(user.value))
    }

    async function removeAvatar() {
        user.value = await authApi.removeAvatar(token.value)
        localStorage.setItem(USER_KEY, JSON.stringify(user.value))
    }

    function clearSession() {
        token.value = ''
        refreshToken.value = ''
        user.value = null
        localStorage.removeItem(TOKEN_KEY)
        localStorage.removeItem(REFRESH_TOKEN_KEY)
        localStorage.removeItem(USER_KEY)
    }

    async function logout() {
        try {
            if (token.value) {
                await authApi.logout(token.value, refreshToken.value)
            }
        } finally {
            clearSession()
        }
    }

    return {
        token,
        refreshToken,
        deviceID,
        user,
        isAuthenticated,
        login,
        register,
        refreshSession,
        restoreUser,
        updateProfile,
        uploadAvatar,
        removeAvatar,
        logout,
    }
})
