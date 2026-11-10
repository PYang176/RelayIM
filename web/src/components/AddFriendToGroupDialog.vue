<script setup lang="ts">
import {computed, nextTick, shallowRef, useTemplateRef, watch} from 'vue'

import type {Conversation} from '@/types/api'

const props = defineProps<{
    open: boolean
    friendName: string
    groups: Conversation[]
    busy: boolean
    error: string
}>()

const emit = defineEmits<{
    close: []
    submit: [payload: {conversationId: string; remark: string}]
}>()

const dialog = useTemplateRef<HTMLDialogElement>('dialog')
const selectInput = useTemplateRef<HTMLSelectElement>('selectInput')
const selectedGroupId = shallowRef('')
const remark = shallowRef('')
const validationError = shallowRef('')
const canSubmit = computed(
    () => Boolean(selectedGroupId.value && remark.value.trim() && !props.busy),
)

watch(
    [() => props.open, () => props.groups],
    async ([open]) => {
        if (!open) {
            dialog.value?.close()
            return
        }
        selectedGroupId.value = props.groups[0]?.id ?? ''
        remark.value = ''
        validationError.value = ''
        if (dialog.value && !dialog.value.open) {
            dialog.value.showModal()
        }
        await nextTick()
        selectInput.value?.focus()
    },
    {immediate: true},
)

// close 关闭添加到群聊弹窗
function close() {
    if (!props.busy) {
        emit('close')
    }
}

// submit 提交群邀请
function submit() {
    if (!selectedGroupId.value) {
        validationError.value = '请选择目标群聊'
        return
    }
    if (!remark.value.trim()) {
        validationError.value = '请填写邀请备注'
        return
    }
    validationError.value = ''
    emit('submit', {
        conversationId: selectedGroupId.value,
        remark: remark.value.trim(),
    })
}
</script>

<template>
    <dialog
            ref="dialog"
            aria-labelledby="add-to-group-title"
            class="add-to-group-dialog"
            @cancel.prevent="close"
            @close="close"
    >
        <form class="add-to-group-card" @submit.prevent="submit">
            <header>
                <div>
                    <h2 id="add-to-group-title">添加到群聊</h2>
                    <p>邀请 {{ friendName }} 加入一个有管理权限的群聊</p>
                </div>
                <button
                        aria-label="关闭添加到群聊"
                        class="icon-button"
                        type="button"
                        @click="close"
                >
                    ×
                </button>
            </header>

            <p v-if="groups.length === 0" class="add-to-group-empty" role="status">
                暂无可管理的群聊。只有群主或管理员可以邀请新成员。
            </p>

            <template v-else>
                <label class="add-to-group-field">
                    <span>目标群聊</span>
                    <select ref="selectInput" v-model="selectedGroupId">
                        <option
                                v-for="group in groups"
                                :key="group.id"
                                :value="group.id"
                        >
                            {{ group.title || group.name }}
                        </option>
                    </select>
                </label>

                <label class="add-to-group-field">
                    <span>邀请备注</span>
                    <textarea
                            v-model="remark"
                            maxlength="200"
                            placeholder="告诉对方为什么邀请加入群聊"
                            rows="3"
                    />
                    <small>{{ remark.trim().length }}/200</small>
                </label>
            </template>

            <p v-if="validationError || error" class="form-error" role="alert">
                {{ validationError || error }}
            </p>

            <footer>
                <button class="ghost-button" type="button" @click="close">
                    取消
                </button>
                <button
                        :disabled="!canSubmit"
                        class="primary-button compact"
                        type="submit"
                >
                    {{ busy ? '发送中…' : '发送邀请' }}
                </button>
            </footer>
        </form>
    </dialog>
</template>

<style scoped>
.add-to-group-dialog {
    width: min(30rem, calc(100vw - 2rem));
    max-width: none;
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-md);
    padding: 0;
    color: var(--ink);
    background: var(--surface);
}

.add-to-group-dialog::backdrop {
    background: rgba(21, 52, 59, 0.42);
}

.add-to-group-card {
    display: grid;
    gap: 1rem;
    padding: 1.25rem;
}

.add-to-group-card header,
.add-to-group-card footer {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 1rem;
}

.add-to-group-card h2,
.add-to-group-card p {
    margin: 0;
}

.add-to-group-card h2 {
    font-size: 1.25rem;
}

.add-to-group-card header p {
    color: var(--muted);
    font-size: 0.75rem;
}

.add-to-group-field {
    display: grid;
    gap: 0.375rem;
    font-size: 0.75rem;
    font-weight: 700;
}

.add-to-group-field select,
.add-to-group-field textarea {
    width: 100%;
    border: 1px solid var(--line);
    border-radius: var(--radius-sm);
    padding: 0.6875rem 0.75rem;
    color: var(--ink);
    background: white;
}

.add-to-group-field textarea {
    resize: vertical;
}

.add-to-group-field small {
    justify-self: end;
    color: var(--muted);
    font-size: 0.625rem;
    font-weight: 400;
}

.add-to-group-empty {
    border: 1px dashed var(--line);
    border-radius: var(--radius-sm);
    padding: 1rem;
    color: var(--muted);
    font-size: 0.8125rem;
}

.add-to-group-card footer {
    justify-content: flex-end;
    border-top: 1px solid var(--line);
    padding-top: 1rem;
}
</style>
