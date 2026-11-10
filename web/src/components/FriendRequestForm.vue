<script setup lang="ts">
import {shallowRef} from 'vue'

const props = defineProps<{
    targetName: string
    busy: boolean
    error: string
}>()

const emit = defineEmits<{
    cancel: []
    submit: [remark: string]
}>()

const remark = shallowRef('')
const validationError = shallowRef('')

// submit 校验申请理由并提交好友申请
function submit() {
    const value = remark.value.trim()
    if (!value) {
        validationError.value = '请填写好友申请理由'
        return
    }
    validationError.value = ''
    emit('submit', value)
}
</script>

<template>
    <form class="friend-request-form" @submit.prevent="submit">
        <header>
            <strong>申请添加 {{ targetName }}</strong>
            <p>填写理由后，对方会在联系人申请中处理。</p>
        </header>
        <label for="friend-request-reason">好友申请理由</label>
        <textarea
                id="friend-request-reason"
                v-model="remark"
                :disabled="busy"
                maxlength="200"
                placeholder="简单说明你为什么要添加对方"
                required
                rows="3"
        />
        <p v-if="validationError || error" class="form-error" role="alert">
            {{ validationError || error }}
        </p>
        <footer>
            <button
                    :disabled="busy"
                    class="text-button"
                    type="button"
                    @click="emit('cancel')"
            >
                取消
            </button>
            <button
                    :disabled="busy || !remark.trim()"
                    class="primary-button compact"
                    type="submit"
            >
                {{ busy ? '发送中…' : '发送申请' }}
            </button>
        </footer>
    </form>
</template>

<style scoped>
.friend-request-form {
    display: grid;
    gap: 0.625rem;
    border: 1px solid var(--line);
    border-radius: var(--radius-sm);
    padding: 0.875rem;
    background: var(--surface);
}

.friend-request-form header strong,
.friend-request-form header p,
.friend-request-form > p {
    margin: 0;
}

.friend-request-form header p {
    margin-top: 0.25rem;
    color: var(--muted);
    font-size: 0.6875rem;
}

.friend-request-form label {
    font-size: 0.75rem;
    font-weight: 700;
}

.friend-request-form textarea {
    width: 100%;
    resize: vertical;
    border: 1px solid var(--line);
    border-radius: var(--radius-sm);
    padding: 0.625rem 0.75rem;
    color: var(--ink);
    background: white;
    font: inherit;
    line-height: 1.6;
}

.friend-request-form textarea:focus {
    border-color: var(--focus);
}

.friend-request-form footer {
    display: flex;
    justify-content: flex-end;
    gap: 0.5rem;
}
</style>
