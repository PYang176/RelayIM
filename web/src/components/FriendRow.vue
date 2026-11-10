<script setup lang="ts">
import {computed, shallowRef, watch} from 'vue'

import type {Friend} from '@/types/api'

const props = defineProps<{
    friend: Friend
    busy: boolean
    selected: boolean
}>()

const emit = defineEmits<{
    select: []
    message: [userID: string]
    updateRemark: [remark: string]
    block: []
    unblock: []
    remove: []
}>()

const editing = shallowRef(false)
const confirmingDelete = shallowRef(false)
const confirmingBlock = shallowRef(false)
const remarkDraft = shallowRef(props.friend.remark)

const displayName = computed(
    () =>
        props.friend.remark ||
        props.friend.user.nickname ||
        props.friend.user.username,
)

watch(
    () => props.friend.remark,
    (remark) => {
        if (!editing.value) {
            remarkDraft.value = remark
        }
    },
)

watch(
    () => props.friend.blocked_by_me,
    (blocked) => {
        if (blocked) {
            confirmingBlock.value = false
        }
    },
)

// openConversation 双击有效好友时打开私聊
function openConversation() {
    if (!props.friend.relationship_active || props.friend.blocked_by_me) return
    emit('message', props.friend.user.id)
}

// saveRemark 提交好友备注更新并退出编辑状态
function saveRemark() {
    emit('updateRemark', remarkDraft.value)
    editing.value = false
}

// cancelEdit 取消备注编辑并恢复原备注
function cancelEdit() {
    remarkDraft.value = props.friend.remark
    editing.value = false
}
</script>

<template>
    <article
            :class="{
                inactive: !friend.relationship_active,
                selected,
            }"
            class="friend-row"
            :title="
                friend.relationship_active && !friend.blocked_by_me
                    ? '双击打开私聊'
                    : undefined
            "
            @click="emit('select')"
            @dblclick="openConversation"
    >
        <button
                class="friend-avatar friend-avatar-button"
                :aria-label="`查看${displayName}的资料`"
                type="button"
                @click.stop="emit('select')"
                @dblclick.stop
        >
            <img
                    v-if="friend.user.avatar_url"
                    :alt="displayName"
                    :src="friend.user.avatar_url"
            />
            <template v-else>{{ displayName.slice(0, 1) }}</template>
        </button>
        <div class="friend-copy">
            <strong>{{ displayName }}</strong>
            <small>@{{ friend.user.username }}</small>
            <p v-if="friend.remark">昵称：{{ friend.user.nickname }}</p>
            <p
                    v-if="!friend.relationship_active"
                    class="friend-relationship-warning"
            >
                双方当前不是好友，无法发送消息
            </p>
        </div>

        <div
                v-if="editing"
                class="friend-inline-form"
                @click.stop
                @dblclick.stop
        >
            <label class="sr-only" :for="`remark-${friend.user.id}`">好友备注</label>
            <input
                    :id="`remark-${friend.user.id}`"
                    v-model="remarkDraft"
                    maxlength="64"
                    placeholder="留空则删除备注"
            />
            <button :disabled="busy" type="button" @click="saveRemark">保存</button>
            <button type="button" @click="cancelEdit">取消</button>
        </div>

        <div
                v-else-if="confirmingDelete"
                class="friend-inline-form"
                @click.stop
                @dblclick.stop
        >
            <span>确认删除好友？</span>
            <button :disabled="busy" type="button" @click="emit('remove')">确认</button>
            <button type="button" @click="confirmingDelete = false">取消</button>
        </div>

        <div
                v-else-if="confirmingBlock"
                class="friend-inline-form"
                @click.stop
                @dblclick.stop
        >
            <span>拉黑后双方将不能私聊或发送好友申请</span>
            <button :disabled="busy" type="button" @click="emit('block')">确认拉黑</button>
            <button type="button" @click="confirmingBlock = false">取消</button>
        </div>

        <div v-else class="friend-actions" @click.stop @dblclick.stop>
            <button
                    v-if="friend.relationship_active && !friend.blocked_by_me"
                    :disabled="busy"
                    type="button"
                    @click="emit('message', friend.user.id)"
            >
                发消息
            </button>
            <button
                    v-if="friend.relationship_active && !friend.blocked_by_me"
                    type="button"
                    @click="editing = true"
            >
                备注
            </button>
            <button
                    v-if="friend.blocked_by_me"
                    :disabled="busy"
                    type="button"
                    @click="emit('unblock')"
            >
                取消拉黑
            </button>
            <button
                    v-else
                    type="button"
                    @click="confirmingBlock = true"
            >
                拉黑
            </button>
            <button type="button" @click="confirmingDelete = true">删除</button>
        </div>
    </article>
</template>

<style scoped>
.friend-row {
    grid-template-columns: 2.5rem minmax(0, 1fr);
    cursor: default;
}

.friend-row:not(.inactive):hover {
    background: color-mix(in srgb, var(--surface-muted) 65%, transparent);
}

.friend-row.selected {
    border-radius: var(--radius-sm);
    background: var(--signal-soft);
}

.friend-row :deep(.friend-actions) {
    grid-column: 2 / -1;
    justify-content: flex-start;
    max-height: 0;
    margin-top: 0;
    overflow: hidden;
    opacity: 0;
    pointer-events: none;
}

.friend-row:hover :deep(.friend-actions),
.friend-row.selected :deep(.friend-actions),
.friend-row :deep(.friend-actions:focus-within) {
    max-height: 5rem;
    margin-top: 0.375rem;
    overflow: visible;
    opacity: 1;
    pointer-events: auto;
}

.friend-avatar-button {
    border: 0;
    padding: 0;
    cursor: pointer;
    overflow: hidden;
}

.friend-avatar-button img {
    width: 100%;
    height: 100%;
    object-fit: cover;
}

.friend-avatar-button:focus-visible {
    outline: 2px solid var(--focus);
    outline-offset: 2px;
}

.friend-relationship-warning {
    color: var(--danger) !important;
    white-space: normal !important;
}
</style>
