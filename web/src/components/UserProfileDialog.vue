<script setup lang="ts">
import {computed, nextTick, shallowRef, useTemplateRef, watch} from 'vue'

import {getUserProfile} from '@/api/auth'
import {useAuthStore} from '@/stores/auth'
import type {UserSummary} from '@/types/api'

const props = defineProps<{
    open: boolean
    userId: string
    canMessage: boolean
}>()

const emit = defineEmits<{
    close: []
    message: [userID: string]
}>()

const auth = useAuthStore()
const dialog = useTemplateRef<HTMLDialogElement>('dialog')
const closeButton = useTemplateRef<HTMLButtonElement>('closeButton')
const user = shallowRef<UserSummary | null>(null)
const loading = shallowRef(false)
const error = shallowRef('')
let requestGeneration = 0

const displayName = computed(
    () => user.value?.nickname || user.value?.username || '用户资料',
)
const avatarLetter = computed(() => displayName.value.slice(0, 1))
const isSelf = computed(() => user.value?.id === auth.user?.id)

watch(
    [() => props.open, () => props.userId],
    async ([open, userId]) => {
        if (!open) {
            dialog.value?.close()
            return
        }
        const generation = ++requestGeneration
        user.value = null
        loading.value = true
        error.value = ''
        if (dialog.value && !dialog.value.open) {
            dialog.value.showModal()
        }
        await nextTick()
        closeButton.value?.focus()
        try {
            const result = await getUserProfile(auth.token, userId)
            if (generation === requestGeneration) {
                user.value = result
            }
        } catch (cause) {
            if (generation === requestGeneration) {
                error.value =
                    cause instanceof Error ? cause.message : '加载用户资料失败'
            }
        } finally {
            if (generation === requestGeneration) {
                loading.value = false
            }
        }
    },
    {immediate: true},
)

// close 关闭只读用户资料弹窗
function close() {
    emit('close')
}

// startMessage 关闭资料弹窗并打开双方私聊
function startMessage() {
    if (!user.value || isSelf.value || !props.canMessage) return
    emit('message', user.value.id)
}
</script>

<template>
    <dialog
            ref="dialog"
            aria-labelledby="user-profile-title"
            class="user-profile-dialog"
            @cancel.prevent="close"
            @close="close"
    >
        <article class="user-profile-card">
            <header class="user-profile-header">
                <span class="user-profile-avatar" aria-hidden="true">
                    <img
                            v-if="user?.avatar_url"
                            :alt="`${displayName}的头像`"
                            :src="user.avatar_url"
                    />
                    <template v-else>{{ avatarLetter }}</template>
                </span>
                <div class="user-profile-heading">
                    <h2 id="user-profile-title">{{ displayName }}</h2>
                    <p v-if="user">@{{ user.username }}</p>
                </div>
                <button
                        ref="closeButton"
                        aria-label="关闭用户资料"
                        class="icon-button"
                        type="button"
                        @click="close"
                >
                    ×
                </button>
            </header>

            <p v-if="loading" class="user-profile-state" role="status">
                正在加载资料…
            </p>
            <p v-else-if="error" class="form-error" role="alert">{{ error }}</p>

            <template v-else-if="user">
                <section class="user-profile-section">
                    <h3>个人简介</h3>
                    <p>{{ user.bio || '这个用户还没有填写个人简介。' }}</p>
                </section>

                <footer class="user-profile-actions">
                    <p v-if="!isSelf && !canMessage">
                        双方当前不是好友，无法继续发送消息。
                    </p>
                    <button
                            v-if="!isSelf && canMessage"
                            class="primary-button compact"
                            type="button"
                            @click="startMessage"
                    >
                        发消息
                    </button>
                    <button class="ghost-button" type="button" @click="close">
                        关闭
                    </button>
                </footer>
            </template>
        </article>
    </dialog>
</template>

<style scoped>
.user-profile-dialog {
    width: min(30rem, calc(100vw - 2rem));
    max-width: none;
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-md);
    padding: 0;
    color: var(--ink);
    background: var(--surface);
}

.user-profile-dialog::backdrop {
    background: rgba(21, 52, 59, 0.42);
}

.user-profile-card {
    display: grid;
    gap: 1.125rem;
    padding: 1.25rem;
}

.user-profile-header {
    display: grid;
    grid-template-columns: 4rem minmax(0, 1fr) auto;
    align-items: center;
    gap: 0.875rem;
}

.user-profile-avatar {
    width: 4rem;
    height: 4rem;
    display: grid;
    place-items: center;
    border-radius: var(--radius-md);
    color: var(--signal-strong);
    background: var(--signal-soft);
    font-family: var(--font-display);
    font-size: 1.5rem;
    font-weight: 600;
    overflow: hidden;
}

.user-profile-avatar img {
    width: 100%;
    height: 100%;
    object-fit: cover;
}

.user-profile-heading {
    min-width: 0;
}

.user-profile-heading h2,
.user-profile-heading p {
    margin: 0;
}

.user-profile-heading h2 {
    font-family: var(--font-display);
    font-size: 1.75rem;
    font-weight: 600;
}

.user-profile-heading p {
    overflow: hidden;
    color: var(--muted);
    font-family: var(--font-data);
    font-size: 0.75rem;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.user-profile-state {
    margin: 0;
    color: var(--muted);
    font-size: 0.8125rem;
}

.user-profile-section {
    border: 1px solid var(--line);
    border-radius: var(--radius-sm);
    padding: 0.875rem;
    background: var(--surface-muted);
}

.user-profile-section h3,
.user-profile-section p {
    margin: 0;
}

.user-profile-section h3 {
    color: var(--muted);
    font-size: 0.75rem;
}

.user-profile-section p {
    margin-top: 0.5rem;
    font-size: 0.8125rem;
    line-height: 1.65;
    white-space: pre-wrap;
}

.user-profile-actions {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 0.5rem;
    border-top: 1px solid var(--line);
    padding-top: 1rem;
}

.user-profile-actions p {
    flex: 1;
    margin: 0;
    color: var(--danger);
    font-size: 0.75rem;
}
</style>
