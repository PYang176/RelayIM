<script setup lang="ts">
import {computed, shallowRef, useTemplateRef, watch} from 'vue'

const props = defineProps<{
    groupName: string
    avatarUrl: string
    busy: boolean
    avatarBusy: boolean
    avatarError: string
}>()

const emit = defineEmits<{
    save: [payload: {name: string}]
    uploadAvatar: [file: File]
    removeAvatar: []
}>()

const avatarInput = useTemplateRef<HTMLInputElement>('avatarInput')
const name = shallowRef('')
const selectionError = shallowRef('')
const avatarLetter = computed(() => props.groupName.trim().slice(0, 1) || '群')

watch(
    () => props.groupName,
    (nextName) => {
        name.value = nextName
    },
    {immediate: true},
)

// submit 校验并提交群名称更新
function submit() {
    if (props.busy || props.avatarBusy || !name.value.trim()) return
    emit('save', {name: name.value.trim()})
}

// chooseAvatar 打开群头像文件选择器
function chooseAvatar() {
    if (!props.avatarBusy) {
        avatarInput.value?.click()
    }
}

// handleAvatar 校验图片类型和大小后提交上传
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
        selectionError.value = '请选择 JPEG、PNG、WebP 或 GIF 图片'
        return
    }
    if (file.size > 5 * 1024 * 1024) {
        selectionError.value = '群头像图片不能超过 5 MB'
        return
    }
    selectionError.value = ''
    emit('uploadAvatar', file)
}
</script>

<template>
    <form class="group-profile-form" @submit.prevent="submit">
        <div class="group-profile-heading">
            <h3>群资料</h3>
            <p>群名称和头像会同步更新到会话列表。</p>
        </div>

        <div class="group-avatar-field">
            <span class="group-avatar-preview">
                <img
                        v-if="avatarUrl"
                        :alt="`${groupName}的群头像`"
                        :src="avatarUrl"
                />
                <template v-else>{{ avatarLetter }}</template>
            </span>
            <div class="group-avatar-copy">
                <strong>群头像</strong>
                <small>支持 JPEG、PNG、WebP、GIF，最大 5 MB。</small>
                <div class="group-avatar-actions">
                    <input
                            ref="avatarInput"
                            class="sr-only"
                            accept="image/jpeg,image/png,image/webp,image/gif"
                            aria-hidden="true"
                            tabindex="-1"
                            type="file"
                            @change="handleAvatar"
                    />
                    <button
                            :disabled="avatarBusy || busy"
                            class="ghost-button"
                            type="button"
                            @click="chooseAvatar"
                    >
                        {{ avatarBusy ? '上传中…' : '上传群头像' }}
                    </button>
                    <button
                            v-if="avatarUrl"
                            :disabled="avatarBusy || busy"
                            class="text-button danger-text"
                            type="button"
                            @click="emit('removeAvatar')"
                    >
                        移除头像
                    </button>
                </div>
            </div>
        </div>

        <p
                v-if="avatarError || selectionError"
                class="form-error"
                role="alert"
        >
            {{ avatarError || selectionError }}
        </p>

        <label>
            <span>群名称</span>
            <input
                    v-model="name"
                    maxlength="64"
                    required
                    type="text"
            />
        </label>

        <button
                :disabled="busy || avatarBusy || !name.trim()"
                class="ghost-button group-profile-submit"
                type="submit"
        >
            {{ busy ? '保存中…' : '保存群名称' }}
        </button>
    </form>
</template>

<style scoped>
.group-profile-form {
    display: grid;
    gap: 0.875rem;
    border: 1px solid var(--line);
    border-radius: var(--radius-md);
    padding: 1rem;
    background: var(--surface-muted);
}

.group-profile-heading h3,
.group-profile-heading p {
    margin: 0;
}

.group-profile-heading h3 {
    font-family: var(--font-display);
    font-size: 1.125rem;
}

.group-profile-heading p {
    margin-top: 0.25rem;
    color: var(--muted);
    font-size: 0.75rem;
}

.group-avatar-field {
    display: flex;
    align-items: center;
    gap: 0.875rem;
}

.group-avatar-preview {
    width: 4rem;
    height: 4rem;
    flex: 0 0 4rem;
    display: grid;
    place-items: center;
    overflow: hidden;
    border: 1px solid var(--line);
    border-radius: 50%;
    color: var(--signal-strong);
    background: var(--signal-soft);
    font-size: 1.25rem;
    font-weight: 800;
}

.group-avatar-preview img {
    width: 100%;
    height: 100%;
    object-fit: cover;
}

.group-avatar-copy {
    min-width: 0;
    display: grid;
    gap: 0.25rem;
}

.group-avatar-copy strong {
    font-size: 0.75rem;
}

.group-avatar-copy small {
    color: var(--muted);
    font-size: 0.625rem;
}

.group-avatar-actions {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.5rem;
    margin-top: 0.25rem;
}

.group-profile-form label {
    display: grid;
    gap: 0.375rem;
}

.group-profile-form label span {
    font-size: 0.75rem;
    font-weight: 700;
}

.group-profile-form input:not([type='file']) {
    width: 100%;
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-sm);
    padding: 0.5rem 0.625rem;
    color: var(--ink);
    background: var(--surface);
}

.group-profile-submit {
    justify-self: end;
}

@media (max-width: 32rem) {
    .group-avatar-field {
        align-items: flex-start;
    }

    .group-avatar-actions {
        align-items: flex-start;
        flex-direction: column;
    }
}
</style>
