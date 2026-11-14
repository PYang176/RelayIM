<script setup lang="ts">
import {computed, nextTick, shallowRef, useTemplateRef, watch} from 'vue'

import GroupGovernancePanel from '@/components/GroupGovernancePanel.vue'
import type {ConversationMember} from '@/types/api'

const props = defineProps<{
    open: boolean
    groupName: string
    members: ConversationMember[]
    currentUserId: string
    loading: boolean
    busy: boolean
    error: string
}>()

const emit = defineEmits<{
    close: []
    remove: [userId: string]
    setRole: [userId: string, role: 1 | 2]
    transferOwner: [userId: string]
    dissolve: []
}>()

const dialog = useTemplateRef<HTMLDialogElement>('dialog')
const closeButton = useTemplateRef<HTMLButtonElement>('closeButton')
const pendingRemovalID = shallowRef('')
const confirmingLeave = shallowRef(false)
const currentMember = computed(() =>
    props.members.find((member) => member.user_id === props.currentUserId),
)
const isOwner = computed(() => currentMember.value?.role === 3)
const canManage = computed(() => (currentMember.value?.role ?? 0) >= 2)

watch(
    () => props.open,
    async (open) => {
        if (open) {
            pendingRemovalID.value = ''
            confirmingLeave.value = false
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

// canRemove 判断当前用户是否可以移除目标成员
function canRemove(member: ConversationMember) {
    if (!canManage.value || member.user_id === props.currentUserId) return false
    if (member.role === 3) return false
    return currentMember.value?.role === 3 || member.role === 1
}

// canAdjustRole 判断当前用户是否可以调整目标成员角色
function canAdjustRole(member: ConversationMember) {
    return isOwner.value &&
        member.user_id !== props.currentUserId &&
        member.role !== 3
}

// toggleRole 切换成员管理员与普通成员角色
function toggleRole(member: ConversationMember) {
    if (props.busy) return
    emit('setRole', member.user_id, member.role === 2 ? 1 : 2)
}

// remove 确认并移除指定群成员
function remove(member: ConversationMember) {
    if (props.busy) return
    if (pendingRemovalID.value !== member.user_id) {
        pendingRemovalID.value = member.user_id
        return
    }
    pendingRemovalID.value = ''
    emit('remove', member.user_id)
}

// leave 提交退出当前群聊操作
function leave() {
    if (props.busy) return
    if (!confirmingLeave.value) {
        confirmingLeave.value = true
        return
    }
    confirmingLeave.value = false
    emit('remove', props.currentUserId)
}

// close 关闭群管理对话框
function close() {
    if (!props.busy) {
        emit('close')
    }
}
</script>

<template>
    <dialog
            ref="dialog"
            class="group-dialog"
            aria-labelledby="group-management-title"
            @cancel.prevent="close"
            @close="close"
    >
        <section class="group-dialog-card">
            <header class="group-dialog-header">
                <div>
                    <h2 id="group-management-title">群管理</h2>
                    <p>{{ groupName }} · 管理成员权限和高风险群组操作。</p>
                </div>
                <button
                        ref="closeButton"
                        class="icon-button"
                        :disabled="busy"
                        aria-label="关闭群管理"
                        type="button"
                        @click="close"
                >
                    ×
                </button>
            </header>

            <p v-if="error" class="form-error" role="alert">{{ error }}</p>
            <p v-if="loading" role="status">加载群成员中…</p>

            <section
                    v-else
                    class="group-management-section"
                    aria-labelledby="group-member-control-title"
            >
                <div class="group-management-heading">
                    <h3 id="group-member-control-title">成员权限</h3>
                    <p>
                        群主可以设置管理员，群主和管理员可以移除普通成员。
                    </p>
                </div>
                <div class="group-member-manage-list">
                    <article
                            v-for="member in members"
                            :key="member.user_id"
                            class="group-member-manage-row"
                    >
                        <div class="group-member-profile-button management-member-copy">
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
                        </div>
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
                        <div
                                v-if="canAdjustRole(member) || canRemove(member)"
                                class="member-controls"
                        >
                            <button
                                    v-if="canAdjustRole(member)"
                                    :disabled="busy"
                                    class="text-button member-role-action"
                                    type="button"
                                    @click="toggleRole(member)"
                            >
                                {{
                                    member.role === 2
                                        ? '取消管理员'
                                        : '设为管理员'
                                }}
                            </button>
                            <button
                                    v-if="canRemove(member)"
                                    :disabled="busy"
                                    class="text-button danger-text member-remove"
                                    type="button"
                                    @click="remove(member)"
                            >
                                {{
                                    pendingRemovalID === member.user_id
                                        ? '确认移除'
                                        : '移除'
                                }}
                            </button>
                        </div>
                    </article>
                </div>
            </section>

            <GroupGovernancePanel
                    v-if="isOwner"
                    :busy="busy"
                    :current-user-id="currentUserId"
                    :members="members"
                    @dissolve="emit('dissolve')"
                    @transfer-owner="emit('transferOwner', $event)"
            />

            <footer class="group-members-footer">
                <span v-if="isOwner">
                    群主可以调整角色、转让群主或解散群聊。
                </span>
                <span v-else-if="canManage">
                    管理员可以移除普通成员，不能调整管理员或群主。
                </span>
                <span v-else>
                    普通成员可以退出群聊，退出后需要重新受邀才能加入。
                </span>
                <button
                        v-if="!isOwner && currentMember"
                        :disabled="busy"
                        class="ghost-button danger-text"
                        type="button"
                        @click="leave"
                >
                    {{ confirmingLeave ? '确认退出群聊' : '退出群聊' }}
                </button>
            </footer>
        </section>
    </dialog>
</template>

<style scoped>
.management-member-copy {
    cursor: default;
}

.management-member-copy:hover {
    background: transparent;
}

.group-management-section {
    display: grid;
    gap: 0.875rem;
    border: 1px solid var(--line);
    border-radius: var(--radius-md);
    padding: 1rem;
}

.group-management-heading h3,
.group-management-heading p {
    margin: 0;
}

.group-management-heading h3 {
    font-size: 1rem;
}

.group-management-heading p {
    margin-top: 0.25rem;
    color: var(--muted);
    font-size: 0.6875rem;
}
</style>
