import {defineStore} from 'pinia'
import {computed, ref} from 'vue'

import * as authApi from '@/api/auth'
import type {UserSummary} from '@/types/api'

const TOKEN_KEY = 'ginchat.access_token'
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
    const user = ref<UserSummary | null>(readStoredUser())

    const isAuthenticated = computed(() => Boolean(token.value))

    function persist(nextToken: string, nextUser: UserSummary) {
        token.value = nextToken
        user.value = nextUser
        localStorage.setItem(TOKEN_KEY, nextToken)
        localStorage.setItem(USER_KEY, JSON.stringify(nextUser))
    }

    async function login(username: string, password: string) {
        const response = await authApi.login(username, password)
        persist(response.token, response.user)
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

    async function restoreUser() {
        if (!token.value) return
        user.value = await authApi.getMe(token.value)
        localStorage.setItem(USER_KEY, JSON.stringify(user.value))
    }

    function logout() {
        token.value = ''
        user.value = null
        localStorage.removeItem(TOKEN_KEY)
        localStorage.removeItem(USER_KEY)
    }

    return {
        token,
        user,
        isAuthenticated,
        login,
        register,
        restoreUser,
        logout,
    }
})
