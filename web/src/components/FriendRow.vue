<script setup lang="ts">
import {computed} from 'vue'

import type {Friend} from '@/types/api'

const props = defineProps<{
    friend: Friend
    selected: boolean
}>()

const emit = defineEmits<{
    select: []
    message: [userID: string]
}>()

const displayName = computed(
    () =>
        props.friend.remark ||
        props.friend.user.nickname ||
        props.friend.user.username,
)

// openConversation 双击有效好友时打开私聊
function openConversation() {
    if (!props.friend.relationship_active || props.friend.blocked_by_me) return
    emit('message', props.friend.user.id)
}
</script>

<template>
    <button
            :class="{
                inactive: !friend.relationship_active,
                selected,
            }"
            :aria-pressed="selected"
            class="friend-row"
            :title="
                friend.relationship_active && !friend.blocked_by_me
                    ? '双击打开私聊'
                    : undefined
            "
            type="button"
            @click="emit('select')"
            @dblclick="openConversation"
    >
        <span class="friend-avatar">
            <img
                    v-if="friend.user.avatar_url"
                    :alt="displayName"
                    :src="friend.user.avatar_url"
            />
            <template v-else>{{ displayName.slice(0, 1) }}</template>
        </span>
        <span class="friend-copy">
            <strong>{{ displayName }}</strong>
            <small>@{{ friend.user.username }}</small>
            <span v-if="friend.remark" class="friend-meta">
                昵称：{{ friend.user.nickname }}
            </span>
            <span v-if="friend.blocked_by_me" class="friend-blocked-state">
                已拉黑，取消后可继续私聊
            </span>
            <span
                    v-if="!friend.relationship_active"
                    class="friend-relationship-warning"
            >
                双方当前不是好友，无法发送消息
            </span>
        </span>
    </button>
</template>

<style scoped>
.friend-row {
    width: 100%;
    grid-template-columns: 2.5rem minmax(0, 1fr);
    border: 0;
    border-bottom: 1px solid var(--line);
    color: var(--ink);
    background: transparent;
    font: inherit;
    text-align: left;
    cursor: pointer;
}

.friend-row:not(.inactive):hover {
    background: color-mix(in srgb, var(--surface-muted) 65%, transparent);
}

.friend-row.selected {
    border-radius: var(--radius-sm);
    background: var(--signal-soft);
}

.friend-avatar {
    overflow: hidden;
}

.friend-avatar img {
    width: 100%;
    height: 100%;
    object-fit: cover;
}

.friend-copy strong,
.friend-copy small,
.friend-copy span {
    overflow: hidden;
    text-overflow: ellipsis;
}

.friend-row:focus-visible {
    border-radius: var(--radius-sm);
    outline: 2px solid var(--focus);
    outline-offset: -2px;
}

.friend-meta {
    margin-top: 0.1875rem;
    color: var(--muted);
    font-size: 0.75rem;
}

.friend-blocked-state {
    color: var(--amber) !important;
    font-size: 0.75rem;
    white-space: normal !important;
}

.friend-relationship-warning {
    color: var(--danger) !important;
    font-size: 0.75rem;
    white-space: normal !important;
}
</style>
