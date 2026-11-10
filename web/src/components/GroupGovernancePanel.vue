<script setup lang="ts">
import {computed, shallowRef, watch} from 'vue'

import type {ConversationMember} from '@/types/api'

const props = defineProps<{
    members: ConversationMember[]
    currentUserId: string
    busy: boolean
}>()

const emit = defineEmits<{
    transferOwner: [userId: string]
    dissolve: []
}>()

const transferTargetId = shallowRef('')
const confirmingTransfer = shallowRef(false)
const confirmingDissolve = shallowRef(false)
const successors = computed(() =>
    props.members.filter((member) => member.user_id !== props.currentUserId),
)

watch(
    successors,
    (members) => {
        if (
            transferTargetId.value &&
            members.some((member) => member.user_id === transferTargetId.value)
        ) {
            return
        }
        transferTargetId.value = members[0]?.user_id ?? ''
        confirmingTransfer.value = false
    },
    {immediate: true},
)

// memberName 计算群成员的展示名称
function memberName(member: ConversationMember) {
    return member.nickname || member.username
}

// selectedMemberName 返回当前转让目标成员的展示名称
function selectedMemberName() {
    const member = successors.value.find(
        (item) => item.user_id === transferTargetId.value,
    )
    return member ? memberName(member) : '所选成员'
}

// changeTransferTarget 更新群主转让目标
function changeTransferTarget() {
    confirmingTransfer.value = false
}

// transferOwner 提交群主转让操作
function transferOwner() {
    if (!transferTargetId.value || props.busy) return
    if (!confirmingTransfer.value) {
        confirmingTransfer.value = true
        return
    }
    confirmingTransfer.value = false
    emit('transferOwner', transferTargetId.value)
}

// dissolve 确认并提交解散群操作
function dissolve() {
    if (props.busy) return
    if (!confirmingDissolve.value) {
        confirmingDissolve.value = true
        return
    }
    confirmingDissolve.value = false
    emit('dissolve')
}
</script>

<template>
    <section class="governance-panel" aria-labelledby="group-governance-title">
        <div class="governance-heading">
            <h3 id="group-governance-title">群主治理</h3>
            <p>高风险操作需要再次确认，完成后成员角色会立即刷新。</p>
        </div>

        <div class="transfer-control">
            <label for="transfer-owner-target">转让群主</label>
            <select
                    id="transfer-owner-target"
                    v-model="transferTargetId"
                    :disabled="busy || successors.length === 0"
                    @change="changeTransferTarget"
            >
                <option
                        v-for="member in successors"
                        :key="member.user_id"
                        :value="member.user_id"
                >
                    {{ memberName(member) }}（{{ member.username }}）
                </option>
            </select>
            <button
                    :disabled="busy || !transferTargetId"
                    class="ghost-button compact-action"
                    type="button"
                    @click="transferOwner"
            >
                {{
                    confirmingTransfer
                        ? `确认转让给 ${selectedMemberName()}`
                        : '转让群主'
                }}
            </button>
        </div>

        <div class="dissolve-control">
            <p>解散后群聊将从所有成员列表隐藏，历史消息保留用于审计。</p>
            <button
                    :disabled="busy"
                    class="ghost-button danger-action"
                    type="button"
                    @click="dissolve"
            >
                {{ confirmingDissolve ? '确认解散群聊' : '解散群聊' }}
            </button>
        </div>
    </section>
</template>

<style scoped>
.governance-panel {
    display: grid;
    gap: 1rem;
    border: 1px solid var(--line);
    border-radius: var(--radius-md);
    padding: 1rem;
    background: var(--surface-muted);
}

.governance-heading {
    display: grid;
    gap: 0.25rem;
}

.governance-heading h3,
.governance-heading p,
.dissolve-control p {
    margin: 0;
}

.governance-heading h3 {
    font-family: var(--font-display);
    font-size: 1.125rem;
}

.governance-heading p,
.dissolve-control p {
    color: var(--muted);
    font-size: 0.75rem;
}

.transfer-control {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr) auto;
    align-items: center;
    gap: 0.75rem;
}

.transfer-control label {
    font-size: 0.75rem;
    font-weight: 700;
}

.transfer-control select {
    min-width: 0;
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-sm);
    padding: 0.5rem 0.625rem;
    color: var(--ink);
    background: var(--surface);
}

.compact-action {
    white-space: nowrap;
}

.dissolve-control {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    border-top: 1px solid var(--line);
    padding-top: 0.75rem;
}

.danger-action {
    color: var(--danger);
    white-space: nowrap;
}

@media (max-width: 36rem) {
    .transfer-control {
        grid-template-columns: 1fr;
    }

    .dissolve-control {
        align-items: stretch;
        flex-direction: column;
    }
}
</style>
