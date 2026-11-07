<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const router = useRouter()
const route = useRoute()

const mode = ref<'login' | 'register'>('login')
const busy = ref(false)
const error = ref('')
const form = reactive({
  username: '',
  password: '',
  confirmPassword: '',
  nickname: '',
})

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

function switchMode(nextMode: 'login' | 'register') {
  mode.value = nextMode
  error.value = ''
}
</script>

<template>
  <main class="auth-page">
    <section class="auth-story">
      <div class="brand-mark">G</div>
      <p class="eyebrow">GO · RABBITMQ · WEBSOCKET</p>
      <h1>更可靠的<br />即时沟通体验</h1>
      <p class="story-copy">
        消息事务、Outbox、实时投递和断线补拉组成一条可验证的聊天链路。
      </p>
      <div class="story-points">
        <span>消息不丢</span>
        <span>多设备送达</span>
        <span>会话内有序</span>
      </div>
    </section>

    <section class="auth-panel">
      <div class="auth-card">
        <div class="mode-tabs">
          <button
            :class="{ active: mode === 'login' }"
            type="button"
            @click="switchMode('login')"
          >
            登录
          </button>
          <button
            :class="{ active: mode === 'register' }"
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
              required
            />
          </label>

          <label v-if="mode === 'register'">
            <span>昵称</span>
            <input v-model="form.nickname" maxlength="64" placeholder="可选" />
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

          <p v-if="error" class="form-error">{{ error }}</p>

          <button class="primary-button" :disabled="busy" type="submit">
            {{ busy ? '处理中…' : mode === 'login' ? '进入 GinChat' : '注册并登录' }}
          </button>
        </form>
      </div>
    </section>
  </main>
</template>
