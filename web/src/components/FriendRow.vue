<script setup lang="ts">
import {shallowRef, watch} from 'vue'

import type {Friend} from '@/types/api'

const props = defineProps<{
    friend: Friend
    busy: boolean
}>()

const emit = defineEmits<{
    message: [userID: string]
    updateRemark: [remark: string]
    remove: []
}>()

const editing = shallowRef(false)
const confirmingDelete = shallowRef(false)
const remarkDraft = shallowRef(props.friend.remark)

watch(
    () => props.friend.remark,
    (remark) => {
        if (!editing.value) {
            remarkDraft.value = remark
        }
    },
)

function displayName() {
    return props.friend.remark || props.friend.user.nickname || props.friend.user.username
}

function saveRemark() {
    emit('updateRemark', remarkDraft.value)
    editing.value = false
}

function cancelEdit() {
    remarkDraft.value = props.friend.remark
    editing.value = false
}
</script>

<template>
    <article class="friend-row">
    <span class="friend-avatar">
      {{ displayName().slice(0, 1) }}
    </span>
        <div class="friend-copy">
            <strong>{{ displayName() }}</strong>
            <small>@{{ friend.user.username }}</small>
            <p v-if="friend.remark">昵称：{{ friend.user.nickname }}</p>
        </div>

        <div v-if="editing" class="friend-inline-form">
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

        <div v-else-if="confirmingDelete" class="friend-inline-form">
            <span>确认删除好友？</span>
            <button :disabled="busy" type="button" @click="emit('remove')">确认</button>
            <button type="button" @click="confirmingDelete = false">取消</button>
        </div>

        <div v-else class="friend-actions">
            <button :disabled="busy" type="button" @click="emit('message', friend.user.id)">
                发消息
            </button>
            <button type="button" @click="editing = true">备注</button>
            <button type="button" @click="confirmingDelete = true">删除</button>
        </div>
    </article>
</template>
