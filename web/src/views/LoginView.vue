<script setup lang="ts">
import {reactive, shallowRef} from 'vue'
import {useRoute, useRouter} from 'vue-router'

import {useAuthStore} from '@/stores/auth'

const auth = useAuthStore()
const router = useRouter()
const route = useRoute()

const mode = shallowRef<'login' | 'register'>('login')
const busy = shallowRef(false)
const error = shallowRef('')
const form = reactive({
    username: '',
    password: '',
    confirmPassword: '',
    nickname: '',
})

// submit 提交登录或注册表单
async function submit() {
    error.value = ''
    busy.value = true
    try {
        if (mode.value === 'register') {
            await auth.register(form)
        } else {
            await auth.login(form.username, form.password)
        }
        const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/'
        await router.replace(redirect)
    } catch (cause) {
        error.value = cause instanceof Error ? cause.message : '认证失败'
    } finally {
        busy.value = false
    }
}

// switchMode 切换登录与注册表单模式
function switchMode(nextMode: 'login' | 'register') {
    mode.value = nextMode
    error.value = ''
}
</script>

<template>
    <main class="auth-page">
        <section class="auth-story">
            <div class="brand-mark">G</div>
            <h1>消息抵达之前，<br/>每一步都有记录</h1>
            <p class="story-copy">
                从会话序号、事务写入，到异步投递和设备确认，RelayIM
                把聊天中容易被忽略的状态做成可追踪的记录。
            </p>
            <ol class="delivery-path" aria-label="一条消息的处理路径">
                <li>
                    <span class="path-seq">01</span>
                    <strong>保存会话消息</strong>
                    <small>客户端幂等键与连续序号同时落库</small>
                </li>
                <li>
                    <span class="path-seq">02</span>
                    <strong>提交待发布事件</strong>
                    <small>消息与 Outbox 在同一个事务中提交</small>
                </li>
                <li>
                    <span class="path-seq">03</span>
                    <strong>分发在线连接</strong>
                    <small>RabbitMQ 消费后写入有界发送队列</small>
                </li>
                <li>
                    <span class="path-seq">04</span>
                    <strong>记录设备确认</strong>
                    <small>送达和已读使用不同的状态语义</small>
                </li>
            </ol>
        </section>

        <section class="auth-panel">
            <div class="auth-card">
                <div class="mode-tabs" role="group" aria-label="认证方式">
                    <button
                            :class="{ active: mode === 'login' }"
                            :aria-pressed="mode === 'login'"
                            type="button"
                            @click="switchMode('login')"
                    >
                        登录
                    </button>
                    <button
                            :class="{ active: mode === 'register' }"
                            :aria-pressed="mode === 'register'"
                            type="button"
                            @click="switchMode('register')"
                    >
                        注册
                    </button>
                </div>

                <h2>{{ mode === 'login' ? '欢迎回来' : '创建账号' }}</h2>
                <p class="muted">
                    {{ mode === 'login' ? '继续你的会话' : '注册后会自动登录' }}
                </p>

                <form @submit.prevent="submit">
                    <label>
                        <span>用户名</span>
                        <input
                                v-model="form.username"
                                autocomplete="username"
                                maxlength="32"
                                minlength="2"
                                autofocus
                                required
                        />
                    </label>

                    <label v-if="mode === 'register'">
                        <span>昵称</span>
                        <input v-model="form.nickname" maxlength="64" placeholder="可选"/>
                    </label>

                    <label>
                        <span>密码</span>
                        <input
                                v-model="form.password"
                                :autocomplete="mode === 'login' ? 'current-password' : 'new-password'"
                                minlength="8"
                                required
                                type="password"
                        />
                    </label>

                    <label v-if="mode === 'register'">
                        <span>确认密码</span>
                        <input
                                v-model="form.confirmPassword"
                                autocomplete="new-password"
                                minlength="8"
                                required
                                type="password"
                        />
                    </label>

                    <p v-if="error" class="form-error" role="alert">{{ error }}</p>

                    <button class="primary-button" :disabled="busy" type="submit">
                        {{ busy ? '处理中…' : mode === 'login' ? '进入 RelayIM' : '注册并登录' }}
                    </button>
                </form>
            </div>
        </section>
    </main>
</template>
