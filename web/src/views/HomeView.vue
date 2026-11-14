<script setup lang="ts">
import {computed, shallowRef} from 'vue'
import {useRoute, useRouter} from 'vue-router'

import AuthDialog from '@/components/AuthDialog.vue'
import WorkspacePreview from '@/components/WorkspacePreview.vue'
import {useAuthStore} from '@/stores/auth'

type AuthMode = 'login' | 'register'

interface AuthSubmission {
    username: string
    password: string
    confirmPassword: string
    nickname: string
}

const auth = useAuthStore()
const router = useRouter()
const route = useRoute()
const busy = shallowRef(false)
const error = shallowRef('')

const authMode = computed<AuthMode | ''>(() => {
    if (route.query.auth === 'register') return 'register'
    if (route.query.auth === 'login') return 'login'
    return ''
})
const authOpen = computed(() => authMode.value !== '')

// authRouteQuery 生成保留原页面参数并切换认证模式的查询参数
function authRouteQuery(mode: AuthMode | '') {
    const query = {...route.query}
    if (mode) {
        query.auth = mode
    } else {
        delete query.auth
    }
    return query
}

// openAuth 打开登录或注册弹窗并同步 URL
function openAuth(mode: AuthMode) {
    error.value = ''
    void router.replace({name: 'home', query: authRouteQuery(mode)})
}

// switchAuthMode 在登录和注册表单之间切换
function switchAuthMode(mode: AuthMode) {
    if (mode !== authMode.value) {
        openAuth(mode)
    }
}

// closeAuth 关闭认证弹窗并回到首页
function closeAuth() {
    error.value = ''
    void router.replace({name: 'home', query: authRouteQuery('')})
}

// submitAuth 提交认证表单并进入应用
async function submitAuth(payload: AuthSubmission) {
    if (!authMode.value) return
    error.value = ''
    busy.value = true
    try {
        if (authMode.value === 'register') {
            await auth.register(payload)
        } else {
            await auth.login(payload.username, payload.password)
        }
        const requestedRedirect =
            typeof route.query.redirect === 'string' &&
            route.query.redirect.startsWith('/')
                ? route.query.redirect
                : '/'
        await router.replace(requestedRedirect)
    } catch (cause) {
        error.value = cause instanceof Error ? cause.message : '认证失败'
    } finally {
        busy.value = false
    }
}
</script>

<template>
    <main class="home-page">
        <div :class="{ 'is-obscured': authOpen }" class="home-stage">
            <header class="home-header">
                <RouterLink class="home-brand" aria-label="RelayIM 首页" to="/">
                    <span class="home-brand-mark">R</span>
                    <span>RelayIM</span>
                </RouterLink>
                <nav class="home-actions" aria-label="账号操作">
                    <button class="home-login-button" type="button" @click="openAuth('login')">
                        登录
                    </button>
                    <button
                            class="home-register-button"
                            type="button"
                            @click="openAuth('register')"
                    >
                        创建账号
                    </button>
                </nav>
            </header>

            <section class="home-hero">
                <div class="home-copy">
                    <h1>和重要的人，随时接上话</h1>
                    <p>消息、群组和文件，都在一个清楚的工作空间里。</p>
                    <div class="home-primary-actions">
                        <button
                                class="home-start-button"
                                type="button"
                                @click="openAuth('register')"
                        >
                            开始使用
                        </button>
                        <button
                                class="home-secondary-button"
                                type="button"
                                @click="openAuth('login')"
                        >
                            已有账号
                        </button>
                    </div>
                    <ul class="home-features" aria-label="产品能力">
                        <li>即时消息</li>
                        <li>群组协作</li>
                        <li>文件分享</li>
                    </ul>
                </div>

                <WorkspacePreview/>
            </section>
        </div>

        <AuthDialog
                :busy="busy"
                :error="error"
                :mode="authMode || 'login'"
                :open="authOpen"
                @close="closeAuth"
                @submit="submitAuth"
                @switch-mode="switchAuthMode"
        />
    </main>
</template>
