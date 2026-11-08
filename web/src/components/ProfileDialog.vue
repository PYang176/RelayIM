<script setup lang="ts">
import {computed, nextTick, shallowRef, useTemplateRef, watch} from 'vue'

import type {UserSummary} from '@/types/api'

const props = defineProps<{
    open: boolean
    user: UserSummary | null
    busy: boolean
    error: string
}>()

const emit = defineEmits<{
    close: []
    save: [payload: {nickname: string; avatarUrl: string; bio: string}]
}>()

const dialog = useTemplateRef<HTMLDialogElement>('dialog')
const nicknameInput = useTemplateRef<HTMLInputElement>('nicknameInput')
const nickname = shallowRef('')
const avatarUrl = shallowRef('')
const bio = shallowRef('')

const displayName = computed(
    () => props.user?.nickname || props.user?.username || '我',
)
const avatarLetter = computed(() => displayName.value.slice(0, 1))

watch(
    () => props.open,
    async (open) => {
        if (!open) {
            dialog.value?.close()
            return
        }
        nickname.value = props.user?.nickname ?? ''
        avatarUrl.value = props.user?.avatar_url ?? ''
        bio.value = props.user?.bio ?? ''
        dialog.value?.showModal()
        await nextTick()
        nicknameInput.value?.focus()
    },
    {immediate: true},
)

function close() {
    if (!props.busy) {
        emit('close')
    }
}

function submit() {
    if (props.busy || !nickname.value.trim()) return
    emit('save', {
        nickname: nickname.value.trim(),
        avatarUrl: avatarUrl.value.trim(),
        bio: bio.value.trim(),
    })
}
</script>

<template>
    <dialog
            ref="dialog"
            class="profile-dialog"
            aria-labelledby="profile-dialog-title"
            @cancel.prevent="close"
            @close="close"
    >
        <form class="profile-card" @submit.prevent="submit">
            <header class="profile-header">
                <div class="profile-identity">
                    <span class="profile-avatar">{{ avatarLetter }}</span>
                    <div>
                        <h2 id="profile-dialog-title">个人资料</h2>
                        <p>@{{ user?.username }}</p>
                    </div>
                </div>
                <button
                        :disabled="busy"
                        class="icon-button"
                        type="button"
                        aria-label="关闭个人资料"
                        @click="close"
                >
                    ×
                </button>
            </header>

            <p v-if="error" class="profile-error" role="alert">{{ error }}</p>

            <label class="profile-field">
                <span>昵称</span>
                <input
                        ref="nicknameInput"
                        v-model="nickname"
                        maxlength="64"
                        required
                        type="text"
                />
                <small>{{ nickname.trim().length }}/64</small>
            </label>

            <label class="profile-field">
                <span>头像地址</span>
                <input
                        v-model="avatarUrl"
                        maxlength="512"
                        placeholder="https://example.com/avatar.png"
                        type="url"
                />
                <small>支持 HTTP 或 HTTPS；留空表示清除头像。</small>
            </label>

            <label class="profile-field">
                <span>个人简介</span>
                <textarea
                        v-model="bio"
                        maxlength="500"
                        rows="4"
                        placeholder="用一句话介绍自己"
                />
                <small>{{ bio.trim().length }}/500</small>
            </label>

            <footer class="profile-actions">
                <button
                        :disabled="busy"
                        class="ghost-button"
                        type="button"
                        @click="close"
                >
                    取消
                </button>
                <button
                        :disabled="busy || !nickname.trim()"
                        class="primary-button compact"
                        type="submit"
                >
                    {{ busy ? '保存中…' : '保存资料' }}
                </button>
            </footer>
        </form>
    </dialog>
</template>

<style scoped>
.profile-dialog {
    width: min(34rem, calc(100vw - 2rem));
    max-width: none;
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-md);
    padding: 0;
    color: var(--ink);
    background: var(--surface);
}

.profile-dialog::backdrop {
    background: rgba(21, 52, 59, 0.42);
}

.profile-card {
    display: grid;
    gap: 1.125rem;
    padding: 1.25rem;
}

.profile-header,
.profile-identity,
.profile-actions {
    display: flex;
    align-items: center;
}

.profile-header,
.profile-actions {
    justify-content: space-between;
    gap: 1rem;
}

.profile-identity {
    gap: 0.75rem;
}

.profile-avatar {
    width: 3rem;
    height: 3rem;
    display: grid;
    place-items: center;
    border-radius: var(--radius-sm);
    color: var(--signal-strong);
    background: var(--signal-soft);
    font-weight: 800;
}

.profile-header h2,
.profile-header p {
    margin: 0;
}

.profile-header h2 {
    font-family: var(--font-display);
    font-size: 1.75rem;
    font-weight: 600;
}

.profile-header p {
    color: var(--muted);
    font-family: var(--font-data);
    font-size: 0.75rem;
}

.profile-error {
    margin: 0;
    color: var(--danger);
    font-size: 0.75rem;
}

.profile-field {
    display: grid;
    gap: 0.375rem;
}

.profile-field > span {
    font-size: 0.75rem;
    font-weight: 700;
}

.profile-field input,
.profile-field textarea {
    width: 100%;
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-sm);
    padding: 0.6875rem 0.8125rem;
    color: var(--ink);
    background: white;
}

.profile-field textarea {
    resize: vertical;
    line-height: 1.55;
}

.profile-field small {
    justify-self: end;
    color: var(--muted);
    font-size: 0.625rem;
}

.profile-actions {
    border-top: 1px solid var(--line);
    padding-top: 1rem;
}

@media (max-width: 36rem) {
    .profile-dialog {
        width: 100vw;
        height: 100dvh;
        max-height: none;
        margin: 0;
        border: 0;
        border-radius: 0;
    }

    .profile-card {
        min-height: 100%;
    }
}
</style>
