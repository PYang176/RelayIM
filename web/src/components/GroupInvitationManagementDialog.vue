<script setup lang="ts">
import {computed, nextTick, shallowRef, useTemplateRef, watch} from 'vue'

import type {
    ConversationMember,
    Friend,
    GroupPendingInvitation,
} from '@/types/api'

const props = defineProps<{
    open: boolean
    groupName: string
    members: ConversationMember[]
    friends: Friend[]
    friendsHasMore: boolean
    loadingMoreFriends: boolean
    invitationHistory: GroupPendingInvitation[]
    invitationHistoryHasMore: boolean
    loadingMoreInvitations: boolean
    currentUserId: string
    busy: boolean
    error: string
}>()

const emit = defineEmits<{
    close: []
    invite: [userIds: string[], inviteRemark: string]
    cancelInvitation: [invitationID: string]
    loadMoreFriends: []
    loadMoreInvitations: []
}>()

const dialog = useTemplateRef<HTMLDialogElement>('dialog')
const closeButton = useTemplateRef<HTMLButtonElement>('closeButton')
const selectedInviteUserIds = shallowRef<string[]>([])
const inviteRemark = shallowRef('')
const currentMember = computed(() =>
    props.members.find((member) => member.user_id === props.currentUserId),
)
const canManage = computed(() => (currentMember.value?.role ?? 0) >= 2)
const activeMemberIDs = computed(
    () => new Set(props.members.map((member) => member.user_id)),
)
const pendingInviteeIDs = computed(
    () => new Set(
        props.invitationHistory
            .filter((invitation) => invitation.status === 1)
            .map((invitation) => invitation.invitee.id),
    ),
)
const availableFriends = computed(() =>
    props.friends.filter(
        (friend) =>
            !activeMemberIDs.value.has(friend.user.id) &&
            !pendingInviteeIDs.value.has(friend.user.id),
    ),
)

watch(
    () => props.open,
    async (open) => {
        if (open) {
            selectedInviteUserIds.value = []
            inviteRemark.value = ''
            dialog.value?.showModal()
            await nextTick()
            closeButton.value?.focus()
        } else {
            dialog.value?.close()
        }
    },
    {immediate: true},
)

// friendName 计算可邀请好友的展示名称
function friendName(friend: Friend) {
    return friend.remark || friend.user.nickname || friend.user.username
}

// invitationStatusLabel 把邀请状态编码转换为中文标签
function invitationStatusLabel(status: number) {
    const labels: Record<number, string> = {
        1: '等待审批',
        2: '已同意',
        3: '已拒绝',
        4: '已过期',
        5: '已取消',
    }
    return labels[status] ?? '未知状态'
}

// invite 提交群成员邀请
function invite() {
    const remark = inviteRemark.value.trim()
    if (
        selectedInviteUserIds.value.length === 0 ||
        !remark ||
        props.busy
    ) {
        return
    }
    emit('invite', [...selectedInviteUserIds.value], remark)
    selectedInviteUserIds.value = []
    inviteRemark.value = ''
}

// close 关闭群邀请管理对话框
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
            aria-labelledby="group-invitation-management-title"
            @cancel.prevent="close"
            @close="close"
    >
        <section class="group-dialog-card">
            <header class="group-dialog-header">
                <div>
                    <h2 id="group-invitation-management-title">群邀请</h2>
                    <p>{{ groupName }} · 邀请好友并跟踪审批状态。</p>
                </div>
                <button
                        ref="closeButton"
                        class="icon-button"
                        :disabled="busy"
                        aria-label="关闭群邀请管理"
                        type="button"
                        @click="close"
                >
                    ×
                </button>
            </header>

            <p v-if="error" class="form-error" role="alert">{{ error }}</p>
            <p v-if="!canManage" role="status">
                只有群主和管理员可以发送或管理群邀请。
            </p>

            <fieldset
                    v-if="canManage"
                    class="group-member-picker invite-picker"
            >
                <legend>
                    邀请好友
                    <span>已选 {{ selectedInviteUserIds.length }} 人</span>
                </legend>
                <p v-if="availableFriends.length === 0 && !friendsHasMore">
                    没有可邀请的好友，群成员和待审批邀请不会重复显示。
                </p>
                <div
                        v-else-if="availableFriends.length > 0"
                        class="group-member-list"
                >
                    <label
                            v-for="friend in availableFriends"
                            :key="friend.friendship_id"
                            class="group-member-option"
                    >
                        <input
                                v-model="selectedInviteUserIds"
                                :value="friend.user.id"
                                type="checkbox"
                        />
                        <span class="friend-avatar">
                            {{ friendName(friend).slice(0, 1) }}
                        </span>
                        <span class="group-member-copy">
                            <strong>{{ friendName(friend) }}</strong>
                            <small>@{{ friend.user.username }}</small>
                        </span>
                    </label>
                </div>
                <button
                        v-if="friendsHasMore"
                        class="text-button"
                        :disabled="loadingMoreFriends"
                        type="button"
                        @click="emit('loadMoreFriends')"
                >
                    {{ loadingMoreFriends ? '加载中…' : '加载更多好友' }}
                </button>
                <label class="group-field" for="manage-invite-remark">
                    <span>入群邀请备注</span>
                    <textarea
                            id="manage-invite-remark"
                            v-model="inviteRemark"
                            maxlength="200"
                            placeholder="说明邀请原因，例如：一起参与本周联调"
                            required
                            rows="3"
                    />
                </label>
                <button
                        class="primary-button compact invite-button"
                        :disabled="
                            busy ||
                            selectedInviteUserIds.length === 0 ||
                            !inviteRemark.trim()
                        "
                        type="button"
                        @click="invite"
                >
                    {{
                        busy
                            ? '处理中…'
                            : `发送 ${selectedInviteUserIds.length} 个邀请`
                    }}
                </button>
            </fieldset>

            <section
                    v-if="canManage"
                    class="group-invitation-history"
                    aria-labelledby="group-invitation-history-title"
            >
                <div class="group-invitation-history-heading">
                    <h3 id="group-invitation-history-title">邀请记录</h3>
                    <span>{{ invitationHistory.length }} 条</span>
                </div>
                <p v-if="invitationHistory.length === 0" role="status">
                    当前群还没有邀请记录。
                </p>
                <div v-else class="group-member-list">
                    <div
                            v-for="invitation in invitationHistory"
                            :key="invitation.invitation_id"
                            class="group-member-option invitation-history-row"
                    >
                        <span class="friend-avatar">
                            {{
                                (
                                    invitation.invitee.nickname ||
                                    invitation.invitee.username
                                ).slice(0, 1)
                            }}
                        </span>
                        <span class="group-member-copy">
                            <strong>
                                {{
                                    invitation.invitee.nickname ||
                                    invitation.invitee.username
                                }}
                            </strong>
                            <small>
                                @{{ invitation.invitee.username }} ·
                                {{ invitationStatusLabel(invitation.status) }}
                            </small>
                            <p>邀请备注：{{ invitation.invite_remark }}</p>
                            <p v-if="invitation.response_remark">
                                回复备注：{{ invitation.response_remark }}
                            </p>
                        </span>
                        <button
                                v-if="invitation.status === 1"
                                :disabled="busy"
                                class="text-button danger-text"
                                type="button"
                                @click="
                                    emit(
                                        'cancelInvitation',
                                        invitation.invitation_id,
                                    )
                                "
                        >
                            取消邀请
                        </button>
                    </div>
                </div>
                <button
                        v-if="invitationHistoryHasMore"
                        class="text-button"
                        :disabled="loadingMoreInvitations"
                        type="button"
                        @click="emit('loadMoreInvitations')"
                >
                    {{
                        loadingMoreInvitations
                            ? '加载中…'
                            : '加载更多邀请记录'
                    }}
                </button>
            </section>
        </section>
    </dialog>
</template>

<style scoped>
.group-invitation-history {
    display: grid;
    gap: 0.75rem;
    border: 1px solid var(--line);
    border-radius: var(--radius-md);
    padding: 1rem;
}

.group-invitation-history-heading {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
}

.group-invitation-history-heading h3,
.group-invitation-history-heading span {
    margin: 0;
}

.group-invitation-history-heading h3 {
    font-size: 1rem;
}

.group-invitation-history-heading span,
.group-invitation-history > p {
    color: var(--muted);
    font-size: 0.6875rem;
}
</style>
