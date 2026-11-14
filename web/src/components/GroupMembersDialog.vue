<script setup lang="ts">
import {nextTick, useTemplateRef, watch} from 'vue'

import type {ConversationMember} from '@/types/api'

const props = defineProps<{
    open: boolean
    groupName: string
    members: ConversationMember[]
    loading: boolean
    error: string
}>()

const emit = defineEmits<{
    close: []
    viewProfile: [member: ConversationMember]
}>()

const dialog = useTemplateRef<HTMLDialogElement>('dialog')
const closeButton = useTemplateRef<HTMLButtonElement>('closeButton')

watch(
    () => props.open,
    async (open) => {
        if (open) {
            dialog.value?.showModal()
            await nextTick()
            closeButton.value?.focus()
        } else {
            dialog.value?.close()
        }
    },
    {immediate: true},
)

// memberName 计算群成员昵称或用户名的展示名称
function memberName(member: {nickname: string; username: string}) {
    return member.nickname || member.username
}

// roleLabel 把成员角色编码转换为中文标签
function roleLabel(role: number) {
    if (role === 3) return '群主'
    if (role === 2) return '管理员'
    return '成员'
}

// close 关闭群成员对话框
function close() {
    emit('close')
}
</script>

<template>
    <dialog
            ref="dialog"
            class="group-members-dialog"
            aria-labelledby="group-members-title"
            @cancel.prevent="close"
            @close="close"
    >
        <section class="group-members-card">
            <header class="group-dialog-header">
                <div>
                    <h2 id="group-members-title">群成员</h2>
                    <p>{{ groupName }} · 当前 {{ members.length }} 人</p>
                </div>
                <button
                        ref="closeButton"
                        class="icon-button"
                        aria-label="关闭群成员"
                        type="button"
                        @click="close"
                >
                    ×
                </button>
            </header>

            <p v-if="error" class="form-error" role="alert">{{ error }}</p>
            <p v-if="loading" role="status">加载群成员中…</p>
            <p v-else-if="members.length === 0" role="status">
                当前没有可展示的群成员。
            </p>

            <div v-else class="group-member-manage-list">
                <article
                        v-for="member in members"
                        :key="member.user_id"
                        class="group-member-manage-row member-roster-row"
                >
                    <button
                            :aria-label="`查看${memberName(member)}的资料`"
                            class="group-member-profile-button"
                            type="button"
                            @click="emit('viewProfile', member)"
                    >
                        <span class="friend-avatar">
                            <img
                                    v-if="member.avatar_url"
                                    :alt="memberName(member)"
                                    :src="member.avatar_url"
                            />
                            <template v-else>
                                {{ memberName(member).slice(0, 1) }}
                            </template>
                        </span>
                        <span class="group-member-copy">
                            <strong>{{ memberName(member) }}</strong>
                            <small>@{{ member.username }}</small>
                        </span>
                    </button>
                    <span
                            :class="[
                                'member-role',
                                {
                                    owner: member.role === 3,
                                    admin: member.role === 2,
                                },
                            ]"
                    >
                        {{ roleLabel(member.role) }}
                    </span>
                </article>
            </div>
        </section>
    </dialog>
</template>

<style scoped>
.member-roster-row {
    grid-template-columns: minmax(0, 1fr) auto;
}
</style>
