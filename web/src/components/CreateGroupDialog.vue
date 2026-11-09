<script setup lang="ts">
import {nextTick, shallowRef, useTemplateRef, watch} from 'vue'

import type {Friend} from '@/types/api'

const props = defineProps<{
    open: boolean
    friends: Friend[]
    loading: boolean
    loadingMore: boolean
    hasMore: boolean
    busy: boolean
    error: string
}>()

const emit = defineEmits<{
    close: []
    loadMore: []
    create: [payload: {name: string; memberUserIds: string[]}]
}>()

const dialog = useTemplateRef<HTMLDialogElement>('dialog')
const nameInput = useTemplateRef<HTMLInputElement>('nameInput')
const groupName = shallowRef('')
const selectedUserIds = shallowRef<string[]>([])

watch(
    () => props.open,
    async (open) => {
        if (open) {
            groupName.value = ''
            selectedUserIds.value = []
            dialog.value?.showModal()
            await nextTick()
            nameInput.value?.focus()
        } else {
            dialog.value?.close()
        }
    },
    {immediate: true},
)

function displayName(friend: Friend) {
    return friend.remark || friend.user.nickname || friend.user.username
}

function close() {
    if (!props.busy) {
        emit('close')
    }
}

function submit() {
    const name = groupName.value.trim()
    if (!name || selectedUserIds.value.length === 0 || props.busy) return
    emit('create', {
        name,
        memberUserIds: [...selectedUserIds.value],
    })
}
</script>

<template>
    <dialog
            ref="dialog"
            class="group-dialog"
            aria-labelledby="create-group-title"
            @cancel.prevent="close"
            @close="close"
    >
        <form class="group-dialog-card" @submit.prevent="submit">
            <header class="group-dialog-header">
                <div>
                    <h2 id="create-group-title">创建群聊</h2>
                    <p>选择好友发送入群邀请，好友同意后进入群聊。</p>
                </div>
                <button
                        class="icon-button"
                        :disabled="busy"
                        aria-label="关闭创建群聊"
                        type="button"
                        @click="close"
                >
                    ×
                </button>
            </header>

            <label class="group-field" for="group-name">
                <span>群名称</span>
                <input
                        id="group-name"
                        ref="nameInput"
                        v-model="groupName"
                        maxlength="64"
                        placeholder="例如：后端学习小组"
                        required
                />
            </label>

            <fieldset class="group-member-picker">
                <legend>
                    选择好友
                    <span>已选 {{ selectedUserIds.length }} 人</span>
                </legend>
                <p v-if="loading" role="status">加载好友中…</p>
                <p v-else-if="error" class="form-error" role="alert">{{ error }}</p>
                <p v-else-if="friends.length === 0" role="status">
                    还没有好友，请先在联系人中发送并接受好友申请。
                </p>
                <div v-else class="group-member-list">
                    <label
                            v-for="friend in friends"
                            :key="friend.friendship_id"
                            class="group-member-option"
                    >
                        <input
                                v-model="selectedUserIds"
                                :value="friend.user.id"
                                type="checkbox"
                        />
                        <span class="friend-avatar">
                            {{ displayName(friend).slice(0, 1) }}
                        </span>
                        <span class="group-member-copy">
                            <strong>{{ displayName(friend) }}</strong>
                            <small>@{{ friend.user.username }}</small>
                        </span>
                    </label>
                    <button
                            v-if="hasMore"
                            class="text-button"
                            :disabled="loadingMore"
                            type="button"
                            @click="emit('loadMore')"
                    >
                        {{ loadingMore ? '加载中…' : '加载更多好友' }}
                    </button>
                </div>
            </fieldset>

            <footer class="group-dialog-actions">
                <span>邀请好友 {{ selectedUserIds.length }} 人</span>
                <div>
                    <button
                            class="ghost-button"
                            :disabled="busy"
                            type="button"
                            @click="close"
                    >
                        取消
                    </button>
                    <button
                            class="primary-button compact"
                            :disabled="
                                busy ||
                                !groupName.trim() ||
                                selectedUserIds.length === 0
                            "
                            type="submit"
                    >
                        {{ busy ? '创建中…' : '创建群聊' }}
                    </button>
                </div>
            </footer>
        </form>
    </dialog>
</template>
