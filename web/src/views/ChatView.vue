<script setup lang="ts">
import {
    computed,
    onBeforeUnmount,
    onMounted,
    shallowRef,
    useTemplateRef,
} from 'vue'
import {useRouter} from 'vue-router'

import AddFriendToGroupDialog from '@/components/AddFriendToGroupDialog.vue'
import ContactDetailPanel from '@/components/ContactDetailPanel.vue'
import ContactsPanel from '@/components/ContactsPanel.vue'
import ConversationList from '@/components/ConversationList.vue'
import CreateGroupDialog from '@/components/CreateGroupDialog.vue'
import GroupInvitationsDialog from '@/components/GroupInvitationsDialog.vue'
import GroupMembersDialog from '@/components/GroupMembersDialog.vue'
import MessageComposer from '@/components/MessageComposer.vue'
import MessageHistoryDialog from '@/components/MessageHistoryDialog.vue'
import MessageList from '@/components/MessageList.vue'
import ProfileDialog from '@/components/ProfileDialog.vue'
import {useAuthStore} from '@/stores/auth'
import {useChatStore} from '@/stores/chat'
import {useFriendshipStore} from '@/stores/friendships'
import {useGroupInvitationStore} from '@/stores/group-invitations'
import type {Friend} from '@/types/api'

const auth = useAuthStore()
const chat = useChatStore()
const friendships = useFriendshipStore()
const groupInvitations = useGroupInvitationStore()
const router = useRouter()
const messageList = useTemplateRef<InstanceType<typeof MessageList>>('messageList')
const workspaceMode = shallowRef<'messages' | 'contacts'>('messages')
const selectedFriendID = shallowRef('')
const groupInvitationsOpen = shallowRef(false)
const groupDialogOpen = shallowRef(false)
const creatingGroup = shallowRef(false)
const groupMembersOpen = shallowRef(false)
const managingMembers = shallowRef(false)
const memberActionError = shallowRef('')
const profileDialogOpen = shallowRef(false)
const savingProfile = shallowRef(false)
const profileError = shallowRef('')
const avatarBusy = shallowRef(false)
const avatarError = shallowRef('')
const messageHistoryOpen = shallowRef(false)
const addToGroupOpen = shallowRef(false)
const addToGroupFriend = shallowRef<Friend | null>(null)
const addToGroupBusy = shallowRef(false)
const addToGroupError = shallowRef('')
let presenceTimer = 0

const connectionLabel = computed(() => {
    const labels = {
        connected: '实时连接正常',
        connecting: '正在连接',
        reconnecting: '正在重连',
        disconnected: '连接已断开',
    }
    return labels[chat.connectionState]
})
const visibleGroupMembers = computed(() => chat.activeMembers.slice(0, 6))
const hiddenGroupMemberCount = computed(() =>
    Math.max(0, chat.activeMembers.length - visibleGroupMembers.value.length),
)
const groupReadLabel = computed(() => {
    const conversation = chat.activeConversation
    if (!conversation) return ''
    const otherMemberCount = Math.max(0, conversation.member_count - 1)
    const readCount = chat.activeReadStates.filter(
        (state) => state.last_read_seq >= conversation.last_seq,
    ).length
    return `${readCount}/${otherMemberCount} 人已读`
})
const activeConversationTitle = computed(() =>
    chat.activeConversation
        ? chat.conversationTitle(chat.activeConversation)
        : '',
)
const activePresenceLabel = computed(() => {
    const conversation = chat.activeConversation
    if (!conversation) return ''
    if (conversation.conversation_type === 1) {
        const peerUserID = conversation.peer_user_id
        if (!peerUserID) return ''
        if (!(peerUserID in chat.presenceByUser)) return '状态查询中'
        return chat.presenceByUser[peerUserID] ? '在线' : '离线'
    }
    const otherMembers = chat.activeMembers.filter(
        (member) => member.user_id !== auth.user?.id,
    )
    if (otherMembers.length === 0) return ''
    if (otherMembers.some((member) => !(member.user_id in chat.presenceByUser))) {
        return '状态查询中'
    }
    const onlineCount = otherMembers.filter(
        (member) => chat.presenceByUser[member.user_id],
    ).length
    return `${onlineCount}/${otherMembers.length} 人在线`
})
const currentMemberRole = computed(
    () =>
        chat.activeMembers.find((member) => member.user_id === auth.user?.id)
            ?.role ?? 1,
)
const activeConversationCanSend = computed(() => {
    const conversation = chat.activeConversation
    if (!conversation) return false
    return (
        conversation.conversation_type === 2 ||
        conversation.direct_friendship_active
    )
})
const selectedFriend = computed(
    () =>
        friendships.friends.find(
            (friend) => friend.user.id === selectedFriendID.value,
        ) ?? null,
)
const manageableGroups = computed(() =>
    chat.conversations.filter(
        (conversation) =>
            conversation.conversation_type === 2 &&
            conversation.current_user_role >= 2,
    ),
)

// selectConversation 切换当前会话并同步页面状态
async function selectConversation(conversationId: string) {
    await chat.selectConversation(conversationId)
    messageList.value?.scrollToBottom()
}

// openGroupMembers 打开群成员管理窗口
function openGroupMembers() {
    memberActionError.value = ''
    groupMembersOpen.value = true
    const conversationId = chat.activeConversationId
    if (conversationId) {
        void chat.loadMembers(conversationId, true)
        void groupInvitations.loadGroupInvitations(conversationId, true)
    }
    void friendships.loadAll()
}

// inviteMembers 邀请好友加入当前群聊
async function inviteMembers(userIds: string[], inviteRemark: string) {
    const succeeded = await runMemberAction(
        async () => {
            await chat.inviteGroupMembers(userIds, inviteRemark)
        },
        '邀请群成员失败',
    )
    if (succeeded && chat.activeConversationId) {
        await groupInvitations.loadGroupInvitations(
            chat.activeConversationId,
            true,
        )
    }
}

// loadMoreGroupInvitations 加载更多待处理群邀请
function loadMoreGroupInvitations() {
    if (chat.activeConversationId) {
        void groupInvitations.loadGroupInvitations(
            chat.activeConversationId,
            false,
        )
    }
}

// cancelGroupInvitation 取消指定群邀请
async function cancelGroupInvitation(invitationID: string) {
    if (chat.activeConversationId) {
        await groupInvitations.cancelInvitation(
            chat.activeConversationId,
            invitationID,
        )
    }
}

// handleGroupInvitationAccepted 接受邀请后切换到目标群会话
async function handleGroupInvitationAccepted(conversationID: string) {
    groupInvitationsOpen.value = false
    await chat.loadConversations()
    await chat.selectConversation(conversationID)
    messageList.value?.scrollToBottom()
}

// removeMember 移除当前群成员
async function removeMember(userID: string) {
    await runMemberAction(async () => {
        await chat.removeGroupMember(userID)
        if (userID === auth.user?.id) {
            groupMembersOpen.value = false
        }
    }, '移除群成员失败')
}

// setMemberRole 调整当前群成员角色
async function setMemberRole(userID: string, role: 1 | 2) {
    await runMemberAction(
        () => chat.setGroupMemberRole(userID, role),
        '调整成员角色失败',
    )
}

// transferOwner 转让当前群主身份
async function transferOwner(userID: string) {
    await runMemberAction(
        () => chat.transferGroupOwner(userID),
        '转让群主失败',
    )
}

// dissolveGroup 解散当前群聊
async function dissolveGroup() {
    const dissolved = await runMemberAction(
        () => chat.dissolveActiveGroup(),
        '解散群聊失败',
    )
    if (dissolved) {
        groupMembersOpen.value = false
    }
}

// updateGroupProfile 保存群名称和群头像
async function updateGroupProfile(payload: {
    name: string
    avatarUrl: string
}) {
    await runMemberAction(
        () => chat.updateGroupProfile(payload.name, payload.avatarUrl),
        '更新群资料失败',
    )
}

// runMemberAction 统一执行成员管理操作并处理错误
async function runMemberAction(
    action: () => Promise<void>,
    fallback: string,
) {
    managingMembers.value = true
    memberActionError.value = ''
    try {
        await action()
        messageList.value?.scrollToBottom()
        return true
    } catch (cause) {
        memberActionError.value =
            cause instanceof Error ? cause.message : fallback
        return false
    } finally {
        managingMembers.value = false
    }
}

// openGroupCreator 打开创建群聊对话框
function openGroupCreator() {
    groupDialogOpen.value = true
    void friendships.loadAll()
}

// createGroup 创建群聊并切换到新会话
async function createGroup(payload: {
    name: string
    memberUserIds: string[]
    inviteRemark: string
}) {
    creatingGroup.value = true
    try {
        await chat.createGroup(
            payload.name,
            payload.memberUserIds,
            payload.inviteRemark,
        )
        groupDialogOpen.value = false
        messageList.value?.scrollToBottom()
    } catch (cause) {
        chat.reportError(cause instanceof Error ? cause.message : '创建群聊失败')
    } finally {
        creatingGroup.value = false
    }
}

// memberName 计算群成员展示名称
function memberName(member: {
    nickname: string
    username: string
}) {
    return member.nickname || member.username
}

// sendMessage 发送文本消息并刷新当前会话
async function sendMessage(content: string) {
    try {
        await chat.sendMessage(content)
        messageList.value?.scrollToBottom()
    } catch (cause) {
        chat.reportError(cause instanceof Error ? cause.message : '发送消息失败')
    }
}

// recallMessage 撤回指定消息并刷新状态
async function recallMessage(messageId: string) {
    try {
        await chat.recallMessage(messageId)
    } catch (cause) {
        chat.reportError(cause instanceof Error ? cause.message : '撤回消息失败')
    }
}

// openProfile 打开当前用户资料对话框
function openProfile() {
    profileError.value = ''
    avatarError.value = ''
    profileDialogOpen.value = true
}

// saveProfile 保存个人资料修改
async function saveProfile(payload: {
    nickname: string
    bio: string
}) {
    savingProfile.value = true
    profileError.value = ''
    try {
        await auth.updateProfile(payload)
        profileDialogOpen.value = false
        const conversationId = chat.activeConversationId
        if (chat.activeConversation?.conversation_type === 2 && conversationId) {
            await chat.loadMembers(conversationId, true)
        }
    } catch (cause) {
        profileError.value =
            cause instanceof Error ? cause.message : '保存个人资料失败'
    } finally {
        savingProfile.value = false
    }
}

// uploadAvatar 上传并保存个人头像
async function uploadAvatar(file: File) {
    avatarBusy.value = true
    avatarError.value = ''
    try {
        await auth.uploadAvatar(file)
        const conversationId = chat.activeConversationId
        if (chat.activeConversation?.conversation_type === 2 && conversationId) {
            await chat.loadMembers(conversationId, true)
        }
    } catch (cause) {
        avatarError.value =
            cause instanceof Error ? cause.message : '上传头像失败'
    } finally {
        avatarBusy.value = false
    }
}

// removeAvatar 移除个人头像
async function removeAvatar() {
    avatarBusy.value = true
    avatarError.value = ''
    try {
        await auth.removeAvatar()
        const conversationId = chat.activeConversationId
        if (chat.activeConversation?.conversation_type === 2 && conversationId) {
            await chat.loadMembers(conversationId, true)
        }
    } catch (cause) {
        avatarError.value =
            cause instanceof Error ? cause.message : '移除头像失败'
    } finally {
        avatarBusy.value = false
    }
}

// sendFile 上传附件并发送图片或文件消息
async function sendFile(file: File) {
    try {
        await chat.sendAttachment(file)
        messageList.value?.scrollToBottom()
    } catch (cause) {
        chat.reportError(cause instanceof Error ? cause.message : '发送附件失败')
    }
}

// markVisibleConversationRead 页面重新可见时推进当前会话已读位置
function markVisibleConversationRead() {
    if (document.visibilityState === 'visible') {
        void chat.markRead()
    }
}

// logout 退出登录并返回登录页
async function logout() {
    chat.reset()
    friendships.reset()
    groupInvitations.reset()
    await auth.logout()
    await router.replace('/login')
}

// openFriendDirect 从好友列表打开或创建私聊
async function openFriendDirect(userID: string) {
    try {
        await chat.createDirect(userID)
        workspaceMode.value = 'messages'
        messageList.value?.scrollToBottom()
    } catch (cause) {
        chat.reportError(cause instanceof Error ? cause.message : '打开好友会话失败')
    }
}

// openMessages 切换回会话列表
function openMessages() {
    workspaceMode.value = 'messages'
}

// openContacts 切换到联系人工作区并刷新好友数据
function openContacts() {
    workspaceMode.value = 'contacts'
    void friendships.loadAll()
}

// selectFriend 选择联系人并在右侧显示资料
function selectFriend(userID: string) {
    selectedFriendID.value = userID
}

// handleFriendRemoved 同步本地会话中的单向好友发送权限
function handleFriendRemoved(userID: string) {
    chat.applyFriendRelationshipChange(userID, false)
    if (selectedFriendID.value === userID) {
        selectedFriendID.value = ''
    }
}

// openAddToGroup 为当前联系人打开群聊选择
function openAddToGroup(friend: Friend) {
    addToGroupError.value = ''
    addToGroupFriend.value = friend
    addToGroupOpen.value = true
}

// closeAddToGroup 关闭群聊邀请弹窗
function closeAddToGroup() {
    if (!addToGroupBusy.value) {
        addToGroupOpen.value = false
    }
}

// inviteContactToGroup 邀请联系人加入目标群聊
async function inviteContactToGroup(payload: {
    conversationId: string
    remark: string
}) {
    const friend = addToGroupFriend.value
    if (!friend) return
    addToGroupBusy.value = true
    addToGroupError.value = ''
    try {
        await chat.inviteUserToGroup(
            payload.conversationId,
            friend.user.id,
            payload.remark,
        )
        addToGroupOpen.value = false
    } catch (cause) {
        addToGroupError.value =
            cause instanceof Error ? cause.message : '邀请联系人加入群聊失败'
    } finally {
        addToGroupBusy.value = false
    }
}

onMounted(async () => {
    document.addEventListener('visibilitychange', markVisibleConversationRead)
    try {
        await auth.restoreUser()
    } catch {
        await auth.logout()
        await router.replace('/login')
        return
    }
    await chat.connectWebSocket()
    await groupInvitations.loadReceived()
    presenceTimer = window.setInterval(() => {
        void chat.refreshPresence()
        void chat.reconcileActiveConversation()
        void groupInvitations.loadReceived()
    }, 30_000)
})

onBeforeUnmount(() => {
    window.clearInterval(presenceTimer)
    document.removeEventListener('visibilitychange', markVisibleConversationRead)
    chat.disconnectWebSocket()
})
</script>

<template>
    <main class="chat-page">
        <a class="skip-link" href="#message-log">跳到消息内容</a>
        <aside class="app-rail" aria-label="主导航">
            <div class="rail-brand" title="RelayIM">
                <span class="brand-dot">G</span>
            </div>
            <button
                    class="rail-avatar"
                    :aria-label="`打开${auth.user?.nickname || auth.user?.username || '我的'}资料`"
                    type="button"
                    @click="openProfile"
            >
                <img
                        v-if="auth.user?.avatar_url"
                        :alt="auth.user.nickname || auth.user.username"
                        :src="auth.user.avatar_url"
                />
                <template v-else>
                    {{
                        (auth.user?.nickname || auth.user?.username || '我')
                            .slice(0, 1)
                    }}
                </template>
            </button>
            <nav class="rail-nav" aria-label="功能入口">
                <button
                        :class="{active: workspaceMode === 'messages'}"
                        :aria-current="
                            workspaceMode === 'messages' ? 'page' : undefined
                        "
                        aria-label="消息"
                        class="rail-nav-button"
                        type="button"
                        @click="openMessages"
                >
                    <span class="rail-nav-mark">聊</span>
                    <small>消息</small>
                </button>
                <button
                        :class="{active: workspaceMode === 'contacts'}"
                        :aria-current="
                            workspaceMode === 'contacts' ? 'page' : undefined
                        "
                        aria-label="联系人"
                        class="rail-nav-button"
                        type="button"
                        @click="openContacts"
                >
                    <span class="rail-nav-mark">联</span>
                    <small>联系人</small>
                </button>
                <button
                        aria-label="群邀请"
                        class="rail-nav-button"
                        type="button"
                        @click="groupInvitationsOpen = true"
                >
                    <span class="rail-nav-mark">邀</span>
                    <small>群邀请</small>
                    <span
                            v-if="groupInvitations.invitations.length"
                            class="rail-badge"
                    >
                        {{ groupInvitations.invitations.length
                        }}{{ groupInvitations.hasMore ? '+' : '' }}
                    </span>
                </button>
            </nav>
            <div class="rail-footer">
                <span
                        :aria-label="connectionLabel"
                        :class="['rail-connection', chat.connectionState]"
                        :title="connectionLabel"
                />
                <button
                        class="rail-logout"
                        aria-label="退出登录"
                        title="退出登录"
                        type="button"
                        @click="logout"
                >
                    退
                </button>
            </div>
        </aside>

        <div class="chat-shell">
            <div v-if="chat.error" class="global-error" role="alert">
                <span>{{ chat.error }}</span>
                <button aria-label="关闭错误提示" type="button" @click="chat.clearError">
                    关闭
                </button>
            </div>

            <section v-if="workspaceMode === 'messages'" class="chat-layout">
                <ConversationList
                        :active-id="chat.activeConversationId"
                        :conversations="chat.displayedConversations"
                        :has-more="chat.hasMoreDisplayedConversations"
                        :loading="chat.loadingConversations"
                        :loading-more="chat.loadingMoreConversations"
                        :search-query="chat.conversationSearchQuery"
                        :searching="chat.searchingConversations"
                        @clear-search="chat.clearConversationSearch"
                        @create-group="openGroupCreator"
                        @load-more="chat.loadMoreConversations"
                        @open-friends="openContacts"
                        @search="chat.searchConversations"
                        @select="selectConversation"
                        @toggle-pin="chat.setConversationPinned"
                />

                <section class="conversation-main">
                    <template v-if="chat.activeConversation">
                        <header class="conversation-header">
                            <div>
                                <h1>{{ chat.conversationTitle(chat.activeConversation) }}</h1>
                                <p>
                                    最后序号 {{ chat.activeConversation.last_seq }}
                                    <span
                                            v-if="
                                                chat.activeConversation.conversation_type === 2
                                            "
                                    >
                                        · {{ chat.activeConversation.member_count }} 位成员
                                    </span>
                                    <span
                                            v-if="
                                                chat.activeConversation.conversation_type === 2 &&
                                                chat.activeConversation.last_seq > 0
                                            "
                                    >
                                        · {{ groupReadLabel }}
                                    </span>
                                    <span
                                            v-else-if="
                                                chat.activeConversation.conversation_type === 1 &&
                                                chat.activeReadStates.length
                                            "
                                    >
                      · 对方已读 {{ chat.activeReadStates[0].last_read_seq }}
                                    </span>
                                    <span v-if="activePresenceLabel">
                                        · {{ activePresenceLabel }}
                                    </span>
                                </p>
                            </div>
                            <button
                                    class="ghost-button"
                                    type="button"
                                    @click="messageHistoryOpen = true"
                            >
                                聊天记录
                            </button>
                        </header>

                        <div
                                v-if="
                                    chat.activeConversation.conversation_type === 1 &&
                                    !activeConversationCanSend
                                "
                                class="relationship-warning"
                                role="status"
                        >
                            双方当前不是好友，无法继续发送消息。你仍可以查看历史记录。
                        </div>

                        <div
                                v-if="chat.activeConversation.conversation_type === 2"
                                class="group-member-strip"
                                aria-label="群成员"
                        >
                            <span class="group-member-label">
                                <span v-if="chat.loadingMembers">成员加载中</span>
                                <span v-else>群成员</span>
                            </span>
                            <span
                                    v-for="member in visibleGroupMembers"
                                    :key="member.user_id"
                                    class="group-member-chip"
                            >
                                {{ memberName(member) }}
                                <small v-if="member.role === 3">群主</small>
                            </span>
                            <span
                                    v-if="hiddenGroupMemberCount > 0"
                                    class="group-member-chip more"
                            >
                                +{{ hiddenGroupMemberCount }}
                            </span>
                            <button
                                    class="text-button group-manage-button"
                                    type="button"
                                    @click="openGroupMembers"
                            >
                                管理成员
                            </button>
                        </div>

                        <MessageList
                                ref="messageList"
                                :conversation-type="
                                    chat.activeConversation?.conversation_type ?? 1
                                "
                                :current-user-id="auth.user?.id ?? ''"
                                :current-user-role="currentMemberRole"
                                :has-more="Boolean(chat.hasMoreByConversation[chat.activeConversationId])"
                                :loading="chat.loadingMessages"
                                :messages="chat.activeMessages"
                                @load-older="chat.loadOlderMessages"
                                @recall="recallMessage"
                        />
                        <MessageComposer
                                :disabled="
                                    chat.connectionState !== 'connected' ||
                                    !activeConversationCanSend
                                "
                                :upload-progress="chat.uploadProgress"
                                :uploading="chat.uploadingAttachment"
                                @send-file="sendFile"
                                @send="sendMessage"
                        />
                    </template>

                    <div v-else class="empty-workspace">
                        <span class="empty-logo">G</span>
                        <h1>选择一个会话</h1>
                        <p>使用左侧“+”创建私聊，或点击“群”邀请好友创建群聊。</p>
                    </div>
                </section>
            </section>

            <section v-else class="chat-layout contacts-workspace">
                <ContactsPanel
                        :selected-friend-id="selectedFriendID"
                        @friend-removed="handleFriendRemoved"
                        @open-direct="openFriendDirect"
                        @select-friend="selectFriend"
                />
                <ContactDetailPanel
                        :friend="selectedFriend"
                        :groups="manageableGroups"
                        @add-to-group="openAddToGroup"
                        @message="openFriendDirect"
                />
            </section>
        </div>

        <GroupInvitationsDialog
                :open="groupInvitationsOpen"
                @close="groupInvitationsOpen = false"
                @accepted="handleGroupInvitationAccepted"
        />

        <CreateGroupDialog
                :busy="creatingGroup"
                :error="friendships.error"
                :friends="friendships.friends"
                :has-more="friendships.friendHasMore"
                :loading="friendships.loading"
                :loading-more="friendships.loadingMore"
                :open="groupDialogOpen"
                @close="groupDialogOpen = false"
                @create="createGroup"
                @load-more="friendships.loadMoreFriends"
        />

        <GroupMembersDialog
                :busy="managingMembers"
                :current-user-id="auth.user?.id ?? ''"
                :error="memberActionError"
                :friends="friendships.friends"
                :friends-has-more="friendships.friendHasMore"
                :group-avatar-url="chat.activeConversation?.avatar_url ?? ''"
                :group-name="activeConversationTitle"
                :loading="chat.loadingMembers"
                :loading-more-friends="friendships.loadingMore"
                :members="chat.activeMembers"
                :open="groupMembersOpen"
                :invitation-history="
                    groupInvitations.groupHistory[chat.activeConversationId] ?? []
                "
                :invitation-history-has-more="
                    groupInvitations.groupHasMore[chat.activeConversationId] ?? false
                "
                :loading-more-invitations="groupInvitations.loadingMore"
                @close="groupMembersOpen = false"
                @dissolve="dissolveGroup"
                @invite="inviteMembers"
                @load-more-friends="friendships.loadMoreFriends"
                @load-more-invitations="loadMoreGroupInvitations"
                @cancel-invitation="cancelGroupInvitation"
                @remove="removeMember"
                @set-role="setMemberRole"
                @transfer-owner="transferOwner"
                @update-profile="updateGroupProfile"
        />

        <MessageHistoryDialog
                :conversation-id="chat.activeConversationId"
                :conversation-title="activeConversationTitle"
                :open="messageHistoryOpen"
                @close="messageHistoryOpen = false"
        />

        <ProfileDialog
                :avatar-busy="avatarBusy"
                :avatar-error="avatarError"
                :busy="savingProfile"
                :error="profileError"
                :open="profileDialogOpen"
                :user="auth.user"
                @close="profileDialogOpen = false"
                @remove-avatar="removeAvatar"
                @save="saveProfile"
                @upload-avatar="uploadAvatar"
        />

        <AddFriendToGroupDialog
                :busy="addToGroupBusy"
                :error="addToGroupError"
                :friend-name="
                    addToGroupFriend?.remark ||
                    addToGroupFriend?.user.nickname ||
                    addToGroupFriend?.user.username ||
                    ''
                "
                :groups="manageableGroups"
                :open="addToGroupOpen"
                @close="closeAddToGroup"
                @submit="inviteContactToGroup"
        />
    </main>
</template>
