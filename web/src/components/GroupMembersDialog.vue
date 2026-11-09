<script setup lang="ts">
import {computed, nextTick, shallowRef, useTemplateRef, watch} from 'vue'

import GroupGovernancePanel from '@/components/GroupGovernancePanel.vue'
import GroupProfileForm from '@/components/GroupProfileForm.vue'
import type {
    ConversationMember,
    Friend,
    GroupPendingInvitation,
} from '@/types/api'

const props = defineProps<{
    open: boolean
    groupName: string
    groupAvatarUrl: string
    members: ConversationMember[]
    friends: Friend[]
    friendsHasMore: boolean
    loadingMoreFriends: boolean
    pendingInvitations: GroupPendingInvitation[]
    pendingInvitationsHasMore: boolean
    loadingMoreInvitations: boolean
    currentUserId: string
    loading: boolean
    busy: boolean
    error: string
}>()

const emit = defineEmits<{
    close: []
    loadMoreFriends: []
    loadMoreInvitations: []
    invite: [userIds: string[]]
    remove: [userId: string]
    setRole: [userId: string, role: 1 | 2]
    transferOwner: [userId: string]
    dissolve: []
    updateProfile: [payload: {name: string; avatarUrl: string}]
}>()

const dialog = useTemplateRef<HTMLDialogElement>('dialog')
const closeButton = useTemplateRef<HTMLButtonElement>('closeButton')
const selectedInviteUserIds = shallowRef<string[]>([])
const pendingRemovalID = shallowRef('')
const confirmingLeave = shallowRef(false)

const currentMember = computed(() =>
    props.members.find((member) => member.user_id === props.currentUserId),
)
const canManage = computed(() => (currentMember.value?.role ?? 0) >= 2)
const isOwner = computed(() => currentMember.value?.role === 3)
const activeMemberIDs = computed(
    () => new Set(props.members.map((member) => member.user_id)),
)
const pendingInviteeIDs = computed(
    () => new Set(
        props.pendingInvitations.map(
            (invitation) => invitation.invitee.id,
        ),
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

function displayName(member: {nickname: string; username: string}) {
    return member.nickname || member.username
}

function friendName(friend: Friend) {
    return friend.remark || friend.user.nickname || friend.user.username
}

function roleLabel(role: number) {
    if (role === 3) return '群主'
    if (role === 2) return '管理员'
    return '成员'
}

function canRemove(member: ConversationMember) {
    if (!canManage.value || member.user_id === props.currentUserId) return false
    if (member.role === 3) return false
    return currentMember.value?.role === 3 || member.role === 1
}

function canAdjustRole(member: ConversationMember) {
    return isOwner.value &&
        member.user_id !== props.currentUserId &&
        member.role !== 3
}

function toggleRole(member: ConversationMember) {
    if (props.busy) return
    emit('setRole', member.user_id, member.role === 2 ? 1 : 2)
}

function close() {
    if (!props.busy) {
        emit('close')
    }
}

function invite() {
    if (selectedInviteUserIds.value.length === 0 || props.busy) return
    emit('invite', [...selectedInviteUserIds.value])
    selectedInviteUserIds.value = []
}

function remove(member: ConversationMember) {
    if (props.busy) return
    if (pendingRemovalID.value !== member.user_id) {
        pendingRemovalID.value = member.user_id
        return
    }
    pendingRemovalID.value = ''
    emit('remove', member.user_id)
}

function leave() {
    if (props.busy) return
    if (!confirmingLeave.value) {
        confirmingLeave.value = true
        return
    }
    confirmingLeave.value = false
    emit('remove', props.currentUserId)
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
                        :disabled="busy"
                        aria-label="关闭群成员"
                        type="button"
                        @click="close"
                >
                    ×
                </button>
            </header>

            <p v-if="error" class="form-error" role="alert">{{ error }}</p>
            <p v-if="loading" role="status">加载群成员中…</p>

            <div v-else class="group-member-manage-list">
                <article
                        v-for="member in members"
                        :key="member.user_id"
                        class="group-member-manage-row"
                >
                    <span class="friend-avatar">
                        {{ displayName(member).slice(0, 1) }}
                    </span>
                    <span class="group-member-copy">
                        <strong>{{ displayName(member) }}</strong>
                        <small>@{{ member.username }}</small>
                    </span>
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

            <section
                    v-if="canManage && pendingInvitations.length > 0"
                    class="group-member-picker invite-picker"
                    aria-label="待审批群邀请"
            >
                <h3>待审批邀请</h3>
                <div class="group-member-list">
                    <div
                            v-for="invitation in pendingInvitations"
                            :key="invitation.invitation_id"
                            class="group-member-option"
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
                                @{{ invitation.invitee.username }} · 等待审批
                            </small>
                        </span>
                    </div>
                </div>
                <button
                        v-if="pendingInvitationsHasMore"
                        class="text-button"
                        :disabled="loadingMoreInvitations"
                        type="button"
                        @click="emit('loadMoreInvitations')"
                >
                    {{
                        loadingMoreInvitations
                            ? '加载中…'
                            : '加载更多待审批邀请'
                    }}
                </button>
            </section>

            <fieldset v-if="canManage" class="group-member-picker invite-picker">
                <legend>
                    邀请好友
                    <span>已选 {{ selectedInviteUserIds.length }} 人</span>
                </legend>
                <p v-if="availableFriends.length === 0 && !friendsHasMore">
                    没有可邀请的好友，群成员和待审批邀请不会重复显示。
                </p>
                <div v-else-if="availableFriends.length > 0" class="group-member-list">
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
                <button
                        class="primary-button compact invite-button"
                        :disabled="busy || selectedInviteUserIds.length === 0"
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

            <GroupProfileForm
                    v-if="isOwner"
                    :avatar-url="groupAvatarUrl"
                    :busy="busy"
                    :group-name="groupName"
                    @save="emit('updateProfile', $event)"
            />

            <GroupGovernancePanel
                    v-if="isOwner"
                    :busy="busy"
                    :current-user-id="currentUserId"
                    :members="members"
                    @dissolve="emit('dissolve')"
                    @transfer-owner="emit('transferOwner', $event)"
            />

            <footer class="group-members-footer">
                <span v-if="isOwner">群主可邀请成员、调整角色或完成群主治理。</span>
                <span v-else>退出后将从会话列表移除，重新受邀后可再次加入。</span>
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
