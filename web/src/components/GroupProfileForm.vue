<script setup lang="ts">
import {shallowRef, watch} from 'vue'

const props = defineProps<{
    groupName: string
    avatarUrl: string
    busy: boolean
}>()

const emit = defineEmits<{
    save: [payload: {name: string; avatarUrl: string}]
}>()

const name = shallowRef('')
const avatarUrl = shallowRef('')

watch(
    () => [props.groupName, props.avatarUrl],
    ([nextName, nextAvatarURL]) => {
        name.value = nextName
        avatarUrl.value = nextAvatarURL
    },
    {immediate: true},
)

function submit() {
    if (props.busy || !name.value.trim()) return
    emit('save', {
        name: name.value.trim(),
        avatarUrl: avatarUrl.value.trim(),
    })
}
</script>

<template>
    <form class="group-profile-form" @submit.prevent="submit">
        <div class="group-profile-heading">
            <h3>群资料</h3>
            <p>名称会立即更新会话列表，头像地址支持 HTTP 或 HTTPS。</p>
        </div>
        <label>
            <span>群名称</span>
            <input
                    v-model="name"
                    maxlength="64"
                    required
                    type="text"
            />
        </label>
        <label>
            <span>群头像地址</span>
            <input
                    v-model="avatarUrl"
                    maxlength="512"
                    placeholder="https://example.com/group.png"
                    type="url"
            />
        </label>
        <button
                :disabled="busy || !name.trim()"
                class="ghost-button group-profile-submit"
                type="submit"
        >
            {{ busy ? '保存中…' : '保存群资料' }}
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

.group-profile-form label {
    display: grid;
    gap: 0.375rem;
}

.group-profile-form label span {
    font-size: 0.75rem;
    font-weight: 700;
}

.group-profile-form input {
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
</style>
