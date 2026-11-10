<script setup lang="ts">
import {computed, nextTick, shallowRef, useTemplateRef, watch} from 'vue'

import type {UserSummary} from '@/types/api'

const props = defineProps<{
    open: boolean
    user: UserSummary | null
    busy: boolean
    avatarBusy: boolean
    error: string
    avatarError: string
}>()

const emit = defineEmits<{
    close: []
    save: [payload: {nickname: string; bio: string}]
    uploadAvatar: [file: File]
    removeAvatar: []
}>()

const dialog = useTemplateRef<HTMLDialogElement>('dialog')
const nicknameInput = useTemplateRef<HTMLInputElement>('nicknameInput')
const avatarInput = useTemplateRef<HTMLInputElement>('avatarInput')
const nickname = shallowRef('')
const bio = shallowRef('')
const avatarSelectionError = shallowRef('')

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
        bio.value = props.user?.bio ?? ''
        avatarSelectionError.value = ''
        dialog.value?.showModal()
        await nextTick()
        nicknameInput.value?.focus()
    },
    {immediate: true},
)

// close 关闭个人资料对话框
function close() {
    if (!props.busy && !props.avatarBusy) {
        emit('close')
    }
}

// submit 提交个人资料修改
function submit() {
    if (props.busy || !nickname.value.trim()) return
    emit('save', {
        nickname: nickname.value.trim(),
        bio: bio.value.trim(),
    })
}

// chooseAvatar 打开头像文件选择器
function chooseAvatar() {
    if (!props.avatarBusy) {
        avatarInput.value?.click()
    }
}

// handleAvatar 选择头像文件并触发上传
function handleAvatar(event: Event) {
    const input = event.target as HTMLInputElement
    const file = input.files?.[0]
    input.value = ''
    if (!file) return
    const allowedTypes = new Set([
        'image/jpeg',
        'image/png',
        'image/webp',
        'image/gif',
    ])
    if (!allowedTypes.has(file.type)) {
        avatarSelectionError.value = '请选择 JPEG、PNG、WebP 或 GIF 图片'
        return
    }
    if (file.size > 5 * 1024 * 1024) {
        avatarSelectionError.value = '头像图片不能超过 5 MB'
        return
    }
    avatarSelectionError.value = ''
    emit('uploadAvatar', file)
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
                    <span class="profile-avatar">
                        <img
                                v-if="user?.avatar_url"
                                :alt="`${displayName}的头像`"
                                :src="user.avatar_url"
                        />
                        <template v-else>{{ avatarLetter }}</template>
                    </span>
                    <div>
                        <h2 id="profile-dialog-title">个人资料</h2>
                        <p>@{{ user?.username }}</p>
                    </div>
                </div>
                <button
                        :disabled="busy || avatarBusy"
                        class="icon-button"
                        type="button"
                        aria-label="关闭个人资料"
                        @click="close"
                >
                    ×
                </button>
            </header>

            <p v-if="error" class="profile-error" role="alert">{{ error }}</p>
            <p v-if="avatarError || avatarSelectionError" class="profile-error" role="alert">
                {{ avatarError || avatarSelectionError }}
            </p>

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

            <div class="profile-avatar-field">
                <div>
                    <strong>头像</strong>
                    <small>支持 JPEG、PNG、WebP、GIF，最大 5 MB。</small>
                </div>
                <div class="profile-avatar-actions">
                    <input
                            ref="avatarInput"
                            class="sr-only"
                            accept="image/jpeg,image/png,image/webp,image/gif"
                            type="file"
                            @change="handleAvatar"
                    />
                    <button
                            :disabled="avatarBusy"
                            class="ghost-button"
                            type="button"
                            @click="chooseAvatar"
                    >
                        {{ avatarBusy ? '上传中…' : '上传图片' }}
                    </button>
                    <button
                            v-if="user?.avatar_url"
                            :disabled="avatarBusy"
                            class="text-button danger-text"
                            type="button"
                            @click="emit('removeAvatar')"
                    >
                        移除头像
                    </button>
                </div>
            </div>

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
                        :disabled="busy || avatarBusy"
                        class="ghost-button"
                        type="button"
                        @click="close"
                >
                    取消
                </button>
                <button
                        :disabled="busy || avatarBusy || !nickname.trim()"
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
    overflow: hidden;
}

.profile-avatar img {
    width: 100%;
    height: 100%;
    object-fit: cover;
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

.profile-avatar-field {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    border: 1px solid var(--line);
    border-radius: var(--radius-sm);
    padding: 0.75rem;
    background: var(--surface-muted);
}

.profile-avatar-field > div:first-child {
    display: grid;
    gap: 0.25rem;
}

.profile-avatar-field strong {
    font-size: 0.75rem;
}

.profile-avatar-field small {
    color: var(--muted);
    font-size: 0.625rem;
}

.profile-avatar-actions {
    display: flex;
    align-items: center;
    gap: 0.5rem;
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

    .profile-avatar-field {
        align-items: flex-start;
        flex-direction: column;
    }

    .profile-avatar-actions {
        width: 100%;
        justify-content: flex-end;
    }
}
</style>
