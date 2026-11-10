import {defineStore} from 'pinia'
import {computed, ref} from 'vue'

import * as authApi from '@/api/auth'
import type {UserSummary} from '@/types/api'

const TOKEN_KEY = 'relayim.access_token'
const REFRESH_TOKEN_KEY = 'relayim.refresh_token'
const DEVICE_KEY = 'relayim.device_id'
const DEVICE_IDS_KEY = 'relayim.device_ids'
const USER_KEY = 'relayim.user'

// accountDeviceKey 生成账号维度的设备缓存键
function accountDeviceKey(username: string) {
    return username.trim().toLocaleLowerCase()
}

// readDeviceIDs 读取并校验本地多账号设备 ID 映射
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

// readAccountDeviceID 读取指定账号保存的设备 ID
function readAccountDeviceID(username: string): string {
    return readDeviceIDs()[accountDeviceKey(username)] ?? ''
}

// readStoredUser 从 localStorage 恢复用户摘要
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

    // persist 持久化令牌、设备和用户摘要
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

    // login 调用登录接口并写入认证状态
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

    // register 注册账号并自动建立登录会话
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

    // refreshSession 刷新访问令牌并保持本地会话有效
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

    // restoreUser 使用当前令牌重新拉取用户资料
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

    // updateProfile 更新用户资料并同步本地缓存
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

    // uploadAvatar 上传头像并同步用户摘要
    async function uploadAvatar(file: File) {
        user.value = await authApi.uploadAvatar(token.value, file)
        localStorage.setItem(USER_KEY, JSON.stringify(user.value))
    }

    // removeAvatar 移除头像并同步用户摘要
    async function removeAvatar() {
        user.value = await authApi.removeAvatar(token.value)
        localStorage.setItem(USER_KEY, JSON.stringify(user.value))
    }

    // clearSession 清空认证状态和本地缓存
    function clearSession() {
        token.value = ''
        refreshToken.value = ''
        user.value = null
        localStorage.removeItem(TOKEN_KEY)
        localStorage.removeItem(REFRESH_TOKEN_KEY)
        localStorage.removeItem(USER_KEY)
    }

    // logout 撤销服务端会话并清理本地登录状态
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
