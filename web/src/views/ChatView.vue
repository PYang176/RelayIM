<script setup lang="ts">
import {
    computed,
    onBeforeUnmount,
    onMounted,
    shallowRef,
    useTemplateRef,
} from 'vue'
import {useRouter} from 'vue-router'

import ConversationList from '@/components/ConversationList.vue'
import CreateGroupDialog from '@/components/CreateGroupDialog.vue'
import FriendsDrawer from '@/components/FriendsDrawer.vue'
import GroupMembersDialog from '@/components/GroupMembersDialog.vue'
import MessageComposer from '@/components/MessageComposer.vue'
import MessageList from '@/components/MessageList.vue'
import ProfileDialog from '@/components/ProfileDialog.vue'
import {useAuthStore} from '@/stores/auth'
import {useChatStore} from '@/stores/chat'
import {useFriendshipStore} from '@/stores/friendships'

const auth = useAuthStore()
const chat = useChatStore()
const friendships = useFriendshipStore()
const router = useRouter()
const messageList = useTemplateRef<InstanceType<typeof MessageList>>('messageList')
const friendsOpen = shallowRef(false)
const groupDialogOpen = shallowRef(false)
const creatingGroup = shallowRef(false)
const groupMembersOpen = shallowRef(false)
const managingMembers = shallowRef(false)
const memberActionError = shallowRef('')
const profileDialogOpen = shallowRef(false)
const savingProfile = shallowRef(false)
const profileError = shallowRef('')

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
const currentMemberRole = computed(
    () =>
        chat.activeMembers.find((member) => member.user_id === auth.user?.id)
            ?.role ?? 1,
)

async function selectConversation(conversationId: string) {
    await chat.selectConversation(conversationId)
    messageList.value?.scrollToBottom()
}

function openGroupMembers() {
    memberActionError.value = ''
    groupMembersOpen.value = true
    const conversationId = chat.activeConversationId
    if (conversationId) {
        void chat.loadMembers(conversationId, true)
    }
    void friendships.loadAll()
}

async function inviteMembers(userIds: string[]) {
    await runMemberAction(
        () => chat.inviteGroupMembers(userIds),
        '邀请群成员失败',
    )
}

async function removeMember(userID: string) {
    await runMemberAction(async () => {
        await chat.removeGroupMember(userID)
        if (userID === auth.user?.id) {
            groupMembersOpen.value = false
        }
    }, '移除群成员失败')
}

async function setMemberRole(userID: string, role: 1 | 2) {
    await runMemberAction(
        () => chat.setGroupMemberRole(userID, role),
        '调整成员角色失败',
    )
}

async function transferOwner(userID: string) {
    await runMemberAction(
        () => chat.transferGroupOwner(userID),
        '转让群主失败',
    )
}

async function dissolveGroup() {
    const dissolved = await runMemberAction(
        () => chat.dissolveActiveGroup(),
        '解散群聊失败',
    )
    if (dissolved) {
        groupMembersOpen.value = false
    }
}

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

function openGroupCreator() {
    groupDialogOpen.value = true
    void friendships.loadAll()
}

async function createGroup(payload: {
    name: string
    memberUserIds: string[]
}) {
    creatingGroup.value = true
    try {
        await chat.createGroup(payload.name, payload.memberUserIds)
        groupDialogOpen.value = false
        messageList.value?.scrollToBottom()
    } catch (cause) {
        chat.reportError(cause instanceof Error ? cause.message : '创建群聊失败')
    } finally {
        creatingGroup.value = false
    }
}

function memberName(member: {
    nickname: string
    username: string
}) {
    return member.nickname || member.username
}

async function sendMessage(content: string) {
    try {
        await chat.sendMessage(content)
        messageList.value?.scrollToBottom()
    } catch (cause) {
        chat.reportError(cause instanceof Error ? cause.message : '发送消息失败')
    }
}

async function recallMessage(messageId: string) {
    try {
        await chat.recallMessage(messageId)
    } catch (cause) {
        chat.reportError(cause instanceof Error ? cause.message : '撤回消息失败')
    }
}

function openProfile() {
    profileError.value = ''
    profileDialogOpen.value = true
}

async function saveProfile(payload: {
    nickname: string
    avatarUrl: string
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

async function sendFile(file: File) {
    try {
        await chat.sendAttachment(file)
        messageList.value?.scrollToBottom()
    } catch (cause) {
        chat.reportError(cause instanceof Error ? cause.message : '发送附件失败')
    }
}

function markVisibleConversationRead() {
    if (document.visibilityState === 'visible') {
        void chat.markRead()
    }
}

async function logout() {
    chat.reset()
    friendships.reset()
    await auth.logout()
    await router.replace('/login')
}

async function openFriendDirect(userID: string) {
    friendsOpen.value = false
    try {
        await chat.createDirect(userID)
        messageList.value?.scrollToBottom()
    } catch (cause) {
        chat.reportError(cause instanceof Error ? cause.message : '打开好友会话失败')
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
})

onBeforeUnmount(() => {
    document.removeEventListener('visibilitychange', markVisibleConversationRead)
    chat.disconnectWebSocket()
})
</script>

<template>
    <main class="chat-page">
        <a class="skip-link" href="#message-log">跳到消息内容</a>
        <header class="app-header">
            <div class="app-brand">
                <span class="brand-dot">G</span>
                <div>
                    <strong>GinChat</strong>
                    <small :class="['connection-state', chat.connectionState]">
                        {{ connectionLabel }}
                    </small>
                </div>
            </div>
            <div class="current-user">
                <span class="current-user-avatar" aria-hidden="true">
                    {{
                        (auth.user?.nickname || auth.user?.username || '我')
                            .slice(0, 1)
                    }}
                </span>
                <span class="current-user-copy">
                    <strong>{{ auth.user?.nickname || auth.user?.username }}</strong>
                </span>
                <button class="ghost-button" type="button" @click="openProfile">
                    资料
                </button>
                <button class="ghost-button" type="button" @click="friendsOpen = true">
                    好友
                </button>
                <button class="ghost-button" type="button" @click="logout">退出</button>
            </div>
        </header>

        <div v-if="chat.error" class="global-error" role="alert">
            <span>{{ chat.error }}</span>
            <button aria-label="关闭错误提示" type="button" @click="chat.clearError">
                关闭
            </button>
        </div>

        <section class="chat-layout">
            <ConversationList
                    :active-id="chat.activeConversationId"
                    :conversations="chat.conversations"
                    :loading="chat.loadingConversations"
                    @create-group="openGroupCreator"
                    @open-friends="friendsOpen = true"
                    @select="selectConversation"
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
                            </p>
                        </div>
                    </header>

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
                            :disabled="chat.connectionState !== 'connected'"
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

        <FriendsDrawer
                :open="friendsOpen"
                @close="friendsOpen = false"
                @open-direct="openFriendDirect"
        />

        <CreateGroupDialog
                :busy="creatingGroup"
                :error="friendships.error"
                :friends="friendships.friends"
                :loading="friendships.loading"
                :open="groupDialogOpen"
                @close="groupDialogOpen = false"
                @create="createGroup"
        />

        <GroupMembersDialog
                :busy="managingMembers"
                :current-user-id="auth.user?.id ?? ''"
                :error="memberActionError"
                :friends="friendships.friends"
                :group-name="activeConversationTitle"
                :loading="chat.loadingMembers"
                :members="chat.activeMembers"
                :open="groupMembersOpen"
                @close="groupMembersOpen = false"
                @dissolve="dissolveGroup"
                @invite="inviteMembers"
                @remove="removeMember"
                @set-role="setMemberRole"
                @transfer-owner="transferOwner"
        />

        <ProfileDialog
                :busy="savingProfile"
                :error="profileError"
                :open="profileDialogOpen"
                :user="auth.user"
                @close="profileDialogOpen = false"
                @save="saveProfile"
        />
    </main>
</template>
