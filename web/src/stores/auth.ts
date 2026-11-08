import {defineStore} from 'pinia'
import {computed, ref} from 'vue'

import * as authApi from '@/api/auth'
import type {UserSummary} from '@/types/api'

const TOKEN_KEY = 'ginchat.access_token'
const REFRESH_TOKEN_KEY = 'ginchat.refresh_token'
const DEVICE_KEY = 'ginchat.device_id'
const USER_KEY = 'ginchat.user'

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
        }
        localStorage.setItem(USER_KEY, JSON.stringify(nextUser))
    }

    async function login(username: string, password: string) {
        const response = await authApi.login(username, password, deviceID.value)
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
        avatarUrl: string
        bio: string
    }) {
        user.value = await authApi.updateProfile(token.value, {
            nickname: input.nickname,
            avatar_url: input.avatarUrl,
            bio: input.bio,
        })
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
        logout,
    }
})
