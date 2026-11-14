<script setup lang="ts">
import {
    computed,
    nextTick,
    reactive,
    shallowRef,
    useTemplateRef,
    watch,
} from 'vue'

type AuthMode = 'login' | 'register'

interface AuthSubmission {
    username: string
    password: string
    confirmPassword: string
    nickname: string
}

const props = defineProps<{
    open: boolean
    mode: AuthMode
    busy: boolean
    error: string
}>()
const emit = defineEmits<{
    close: []
    switchMode: [mode: AuthMode]
    submit: [payload: AuthSubmission]
}>()
const authDialog = useTemplateRef<HTMLDialogElement>('authDialog')
const showPassword = shallowRef(false)
const form = reactive<AuthSubmission>({
    username: '',
    password: '',
    confirmPassword: '',
    nickname: '',
})

const authTitle = computed(() =>
    props.mode === 'register' ? '创建你的账号' : '欢迎回来',
)
const authDescription = computed(() =>
    props.mode === 'register'
        ? '注册后会直接进入消息页。'
        : '登录后继续查看你的消息。',
)

// switchMode 请求首页切换当前认证模式
function switchMode(mode: AuthMode) {
    if (mode !== props.mode) {
        emit('switchMode', mode)
    }
}

// closeDialog 清理临时显示状态并请求关闭弹窗
function closeDialog() {
    showPassword.value = false
    emit('close')
}

// handleDialogClick 点击弹窗外遮罩时请求关闭弹窗
function handleDialogClick(event: MouseEvent) {
    if (event.target === event.currentTarget) {
        closeDialog()
    }
}

// submitForm 把表单内容交给首页执行认证请求
function submitForm() {
    emit('submit', {
        username: form.username,
        password: form.password,
        confirmPassword: form.confirmPassword,
        nickname: form.nickname,
    })
}

watch(
    () => props.open,
    async (open) => {
        await nextTick()
        const dialog = authDialog.value
        if (!dialog) return
        if (open && !dialog.open) {
            dialog.showModal()
        } else if (!open && dialog.open) {
            dialog.close()
        }
    },
    {immediate: true},
)
</script>

<template>
    <dialog
            ref="authDialog"
            aria-labelledby="auth-dialog-title"
            class="auth-dialog"
            @cancel.prevent="closeDialog"
            @click="handleDialogClick"
    >
        <section class="auth-dialog-card">
            <header class="auth-dialog-head">
                <div class="auth-dialog-brand">
                    <span class="home-brand-mark">R</span>
                    <strong>RelayIM</strong>
                </div>
                <div class="auth-dialog-actions">
                    <div class="auth-mode-tabs" role="group" aria-label="认证方式">
                        <button
                                :class="{active: mode === 'login'}"
                                :aria-pressed="mode === 'login'"
                                type="button"
                                @click="switchMode('login')"
                        >
                            登录
                        </button>
                        <button
                                :class="{active: mode === 'register'}"
                                :aria-pressed="mode === 'register'"
                                type="button"
                                @click="switchMode('register')"
                        >
                            注册
                        </button>
                    </div>
                    <button
                            aria-label="关闭认证窗口"
                            class="auth-close-button"
                            type="button"
                            @click="closeDialog"
                    >
                        ×
                    </button>
                </div>
            </header>

            <div class="auth-dialog-copy">
                <h2 id="auth-dialog-title">{{ authTitle }}</h2>
                <p>{{ authDescription }}</p>
            </div>

            <form class="auth-form" @submit.prevent="submitForm">
                <label class="auth-field">
                    <span>用户名</span>
                    <input
                            v-model="form.username"
                            autocomplete="username"
                            maxlength="32"
                            minlength="2"
                            autofocus
                            placeholder="输入用户名"
                            required
                    />
                </label>

                <label v-if="mode === 'register'" class="auth-field">
                    <span>昵称</span>
                    <input
                            v-model="form.nickname"
                            maxlength="64"
                            placeholder="别人会这样称呼你（可选）"
                    />
                </label>

                <label class="auth-field">
                    <span>密码</span>
                    <span class="auth-password-field">
                        <input
                                v-model="form.password"
                                :autocomplete="
                                    mode === 'login'
                                        ? 'current-password'
                                        : 'new-password'
                                "
                                :type="showPassword ? 'text' : 'password'"
                                minlength="8"
                                placeholder="至少 8 个字符"
                                required
                        />
                        <button
                                :aria-label="showPassword ? '隐藏密码' : '显示密码'"
                                :aria-pressed="showPassword"
                                type="button"
                                @click="showPassword = !showPassword"
                        >
                            {{ showPassword ? '隐藏' : '显示' }}
                        </button>
                    </span>
                </label>

                <label v-if="mode === 'register'" class="auth-field">
                    <span>确认密码</span>
                    <input
                            v-model="form.confirmPassword"
                            autocomplete="new-password"
                            minlength="8"
                            placeholder="再次输入密码"
                            required
                            type="password"
                    />
                </label>

                <p v-if="error" class="form-error" role="alert">{{ error }}</p>

                <button
                        :aria-label="mode === 'register' ? '创建账号' : '进入 RelayIM'"
                        :disabled="busy"
                        class="auth-submit-button"
                        type="submit"
                >
                    {{
                        busy
                            ? '请稍候…'
                            : mode === 'register'
                              ? '创建账号'
                              : '登录'
                    }}
                </button>
            </form>
        </section>
    </dialog>
</template>
