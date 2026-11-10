import {defineStore} from 'pinia'
import {computed, ref, shallowRef} from 'vue'

import * as conversationApi from '@/api/conversations'
import * as messageApi from '@/api/messages'
import * as presenceApi from '@/api/presence'
import {useChatSocket} from '@/composables/useChatSocket'
import {useAuthStore} from '@/stores/auth'
import {useFriendshipStore} from '@/stores/friendships'
import {useGroupInvitationStore} from '@/stores/group-invitations'
import {
    createSequenceCursor,
    needsLatestWindowSync,
    observeSequence,
    type SequenceCursor,
    type SequenceObservation,
} from '@/stores/message-ordering'
import type {
    Conversation,
    ConversationMember,
    DeliverySummary,
    Message,
    ReadState,
    WsMessage,
} from '@/types/api'

export interface ChatMessage extends Message {
    delivery?: DeliverySummary
    outgoing?: boolean
    realtime_attachment_path?: string
}

interface ConversationSequenceState {
    cursor: SequenceCursor
    pendingMessages: Map<number, ChatMessage>
    repairing: boolean
    resyncing: boolean
    repairRequested: boolean
    retryTimer?: number
    repairAttempts: number
    generation: number
}

export const useChatStore = defineStore('chat', () => {
    const auth = useAuthStore()
    const friendships = useFriendshipStore()
    const groupInvitations = useGroupInvitationStore()
    const conversations = ref<Conversation[]>([])
    const messagesByConversation = ref<Record<string, ChatMessage[]>>({})
    const membersByConversation = ref<Record<string, ConversationMember[]>>({})
    const activeConversationId = shallowRef('')
    const readStates = ref<ReadState[]>([])
    const presenceByUser = ref<Record<string, boolean>>({})
    const loadingConversations = shallowRef(false)
    const loadingMoreConversations = shallowRef(false)
    const conversationNextCursor = shallowRef('')
    const hasMoreConversations = shallowRef(false)
    const conversationSearchQuery = shallowRef('')
    const conversationSearchResults = ref<Conversation[]>([])
    const searchNextCursor = shallowRef('')
    const hasMoreSearchResults = shallowRef(false)
    const searchingConversations = shallowRef(false)
    const loadingMessages = shallowRef(false)
    const loadingMembers = shallowRef(false)
    const uploadingAttachment = shallowRef(false)
    const uploadProgress = shallowRef(0)
    const hasMoreByConversation = ref<Record<string, boolean>>({})
    const error = shallowRef('')
    const sequenceStates = new Map<string, ConversationSequenceState>()

    const activeConversation = computed(() =>
        conversations.value.find((item) => item.id === activeConversationId.value),
    )
    const activeMessages = computed(
        () => messagesByConversation.value[activeConversationId.value] ?? [],
    )
    const activeMembers = computed(
        () => membersByConversation.value[activeConversationId.value] ?? [],
    )
    const activeReadStates = computed(() =>
        readStates.value.filter((item) => item.user_id !== auth.user?.id),
    )
    const displayedConversations = computed(() =>
        conversationSearchQuery.value
            ? conversationSearchResults.value
            : conversations.value,
    )
    const hasMoreDisplayedConversations = computed(() =>
        conversationSearchQuery.value
            ? hasMoreSearchResults.value
            : hasMoreConversations.value,
    )
    let searchGeneration = 0

    const realtime = useChatSocket({
        getToken: () => auth.token,
        onConnected: handleSocketConnected,
        onMessage: handleSocketMessage,
        onError: setError,
    })
    const connectionState = realtime.connectionState

    // setError 设置聊天页面的当前错误信息
    function setError(message: string) {
        error.value = message
    }

    // clearError 清空聊天页面错误信息
    function clearError() {
        error.value = ''
    }

    // sequenceStateFor 获取或初始化指定会话的连续序号状态
    function sequenceStateFor(
        conversationId: string,
        contiguousSeq = 0,
    ): ConversationSequenceState {
        const existing = sequenceStates.get(conversationId)
        if (existing) return existing
        const created: ConversationSequenceState = {
            cursor: createSequenceCursor(contiguousSeq),
            pendingMessages: new Map(),
            repairing: false,
            resyncing: false,
            repairRequested: false,
            repairAttempts: 0,
            generation: 0,
        }
        sequenceStates.set(conversationId, created)
        return created
    }

    // clearSequenceState 清除指定会话的序号和缺口缓存
    function clearSequenceState(conversationId: string) {
        const state = sequenceStates.get(conversationId)
        if (state?.retryTimer) {
            window.clearTimeout(state.retryTimer)
        }
        sequenceStates.delete(conversationId)
    }

    // storeMessage 按消息 ID 去重并写入会话消息列表
    function storeMessage(conversationId: string, message: ChatMessage) {
        upsertMessage(conversationId, message)
        if (message.realtime_attachment_path) {
            void resolveRealtimeAttachment(
                conversationId,
                message.message_id,
                message.realtime_attachment_path,
            )
        }
    }

    // flushPendingMessages 把已补齐缺口的待处理消息写入列表
    function flushPendingMessages(
        conversationId: string,
        state: ConversationSequenceState,
    ) {
        const ready = [...state.pendingMessages.entries()]
            .filter(([seq]) => seq <= state.cursor.contiguousSeq)
            .sort(([left], [right]) => left - right)
        for (const [seq, message] of ready) {
            state.pendingMessages.delete(seq)
            storeMessage(conversationId, message)
        }
    }

    // acceptMessageSequence 推进会话连续序号并检测乱序缺口
    function acceptMessageSequence(
        conversationId: string,
        message: ChatMessage,
    ): SequenceObservation {
        let state = sequenceStates.get(conversationId)
        if (!state) {
            const knownSeq =
                conversations.value.find(
                    (item) => item.id === conversationId,
                )?.last_seq ?? 0
            state = sequenceStateFor(conversationId, knownSeq)
        }
        const previousSeq = state.cursor.contiguousSeq
        const observation = observeSequence(state.cursor, message.seq)
        if (message.seq > previousSeq + 1) {
            state.pendingMessages.set(message.seq, message)
        } else {
            storeMessage(conversationId, message)
            flushPendingMessages(conversationId, state)
        }
        if (observation.gapDetected) {
            scheduleGapRepair(conversationId)
        }
        return observation
    }

    // scheduleGapRepair 为存在缺口的会话安排补拉
    function scheduleGapRepair(conversationId: string, immediate = true) {
        const state = sequenceStateFor(conversationId)
        if (state.resyncing || state.repairing) return
        if (state.retryTimer) return
        const delay = immediate
            ? 0
            : Math.min(500 * 2 ** state.repairAttempts, 5000)
        state.retryTimer = window.setTimeout(() => {
            state.retryTimer = undefined
            void repairConversationGap(conversationId)
        }, delay)
    }

    // repairConversationGap 按连续水位补拉会话缺失消息
    async function repairConversationGap(conversationId: string) {
        const state = sequenceStateFor(conversationId)
        if (state.resyncing) return
        if (state.repairing) {
            state.repairRequested = true
            return
        }
        const generation = state.generation
        state.repairing = true
        try {
            do {
                state.repairRequested = false
                const afterSeq = state.cursor.contiguousSeq
                const page = await messageApi.listMessages(
                    auth.token,
                    conversationId,
                    {
                        afterSeq: afterSeq || undefined,
                        limit: 100,
                    },
                )
                if (
                    sequenceStates.get(conversationId) !== state ||
                    state.generation !== generation
                ) {
                    return
                }
                let pageProgress = false
                for (const message of page.messages) {
                    const duplicate = (
                        messagesByConversation.value[conversationId] ?? []
                    ).some((item) => item.message_id === message.message_id) ||
                        [...state.pendingMessages.values()].some(
                            (item) => item.message_id === message.message_id,
                        )
                    const observation = acceptMessageSequence(
                        conversationId,
                        message,
                    )
                    if (!duplicate && conversationId === activeConversationId.value) {
                        sendAck(message.message_id)
                    }
                    pageProgress = pageProgress || observation.advanced
                }
                if (page.has_more) {
                    if (!pageProgress) {
                        break
                    }
                    state.repairRequested = true
                }
            } while (state.repairRequested)

            if (state.cursor.pendingSeqs.size === 0) {
                state.repairAttempts = 0
                if (
                    conversationId === activeConversationId.value &&
                    document.visibilityState === 'visible'
                ) {
                    await markRead()
                }
            } else {
                state.repairAttempts += 1
                scheduleGapRepair(conversationId, false)
            }
        } catch {
            if (
                sequenceStates.get(conversationId) !== state ||
                state.generation !== generation
            ) {
                return
            }
            state.repairAttempts += 1
            scheduleGapRepair(conversationId, false)
        } finally {
            const stale =
                sequenceStates.get(conversationId) !== state ||
                state.generation !== generation
            if (!stale) {
                state.repairing = false
                if (state.repairRequested && state.cursor.pendingSeqs.size > 0) {
                    scheduleGapRepair(conversationId)
                }
            }
        }
    }

    // conversationTitle 计算会话展示标题
    function conversationTitle(conversation: Conversation): string {
        return conversation.title || conversation.name || '私聊'
    }

    // conversationIDCompare 稳定比较 Snowflake 会话 ID 字符串
    function conversationIDCompare(left: string, right: string): number {
        const leftID = BigInt(left)
        const rightID = BigInt(right)
        if (leftID === rightID) return 0
        return leftID > rightID ? -1 : 1
    }

    // sortConversations 按置顶和最近活动时间排序会话
    function sortConversations(items: Conversation[]): Conversation[] {
        return [...items].sort((left, right) => {
            if (left.pinned !== right.pinned) {
                return left.pinned ? -1 : 1
            }
            const timeDifference =
                Date.parse(right.updated_at) - Date.parse(left.updated_at)
            if (Number.isFinite(timeDifference) && timeDifference !== 0) {
                return timeDifference
            }
            return conversationIDCompare(left.id, right.id)
        })
    }

    // upsertConversation 插入或更新会话摘要并重新排序
    function upsertConversation(conversation: Conversation) {
        conversations.value = upsertConversationInto(
            conversations.value,
            conversation,
        )
        const searchIndex = conversationSearchResults.value.findIndex(
            (item) => item.id === conversation.id,
        )
        if (searchIndex >= 0) {
            const searchResults = [...conversationSearchResults.value]
            searchResults[searchIndex] = {
                ...conversation,
                // 并发刷新可能晚于实时消息返回，会话序号不能因此倒退
                last_seq: Math.max(
                    searchResults[searchIndex].last_seq,
                    conversation.last_seq,
                ),
            }
            conversationSearchResults.value = sortConversations(searchResults)
        }
    }

    // upsertConversationInto 在指定列表中插入或合并会话摘要
    function upsertConversationInto(
        items: Conversation[],
        conversation: Conversation,
    ): Conversation[] {
        const index = items.findIndex((item) => item.id === conversation.id)
        const next = [...items]
        if (index >= 0) {
            next[index] = {
                ...conversation,
                last_seq: Math.max(next[index].last_seq, conversation.last_seq),
            }
        } else {
            next.push(conversation)
        }
        return sortConversations(next)
    }

    // upsertMessage 插入、替换或去重一条消息
    function upsertMessage(conversationId: string, message: ChatMessage) {
        const current = messagesByConversation.value[conversationId] ?? []
        const index = current.findIndex((item) => item.message_id === message.message_id)
        const next = [...current]
        if (index >= 0) {
            next[index] = message.recalled
                ? {
                    ...next[index],
                    ...message,
                    content: '',
                    attachment_id: undefined,
                    attachment_url: undefined,
                    attachment_name: undefined,
                    realtime_attachment_path: undefined,
                }
                : {...next[index], ...message}
        } else {
            next.push(message)
        }
        next.sort((left, right) => left.seq - right.seq)
        messagesByConversation.value = {
            ...messagesByConversation.value,
            [conversationId]: next,
        }
    }

    // markMessageRecalled 更新消息撤回状态并清除正文内容
    function markMessageRecalled(
        conversationId: string,
        messageId: string,
        recalledBy = '',
        recalledAt = new Date().toISOString(),
    ) {
        const sequenceState = sequenceStates.get(conversationId)
        if (sequenceState) {
            for (const [seq, pending] of sequenceState.pendingMessages) {
                if (pending.message_id !== messageId) continue
                sequenceState.pendingMessages.set(seq, {
                    ...pending,
                    content: '',
                    attachment_id: undefined,
                    attachment_url: undefined,
                    attachment_name: undefined,
                    realtime_attachment_path: undefined,
                    recalled: true,
                    recalled_at: recalledAt,
                    recalled_by: recalledBy || pending.recalled_by,
                })
                return
            }
        }
        const current = messagesByConversation.value[conversationId] ?? []
        const index = current.findIndex((item) => item.message_id === messageId)
        if (index < 0) return
        const next = [...current]
        next[index] = {
            ...next[index],
            content: '',
            attachment_id: undefined,
            attachment_url: undefined,
            attachment_name: undefined,
            realtime_attachment_path: undefined,
            recalled: true,
            recalled_at: recalledAt,
            recalled_by: recalledBy || next[index].recalled_by,
        }
        messagesByConversation.value = {
            ...messagesByConversation.value,
            [conversationId]: next,
        }
    }

    // updateConversationFromMessage 根据新消息更新会话摘要和未读数
    function updateConversationFromMessage(message: Message) {
        const matches = [
            conversations.value.find(
                (item) => item.id === message.conversation_id,
            ),
            conversationSearchResults.value.find(
                (item) => item.id === message.conversation_id,
            ),
        ].filter((item): item is Conversation => Boolean(item))
        if (matches.length === 0) {
            void refreshConversation(message.conversation_id)
            return
        }

        const shouldIncrementUnread =
            message.sender_id !== auth.user?.id &&
            (
                message.conversation_id !== activeConversationId.value ||
                document.visibilityState !== 'visible'
            )
        const unreadCount =
            Math.max(...matches.map((item) => item.unread_count)) +
            (shouldIncrementUnread ? 1 : 0)
        for (const conversation of matches) {
            conversation.last_seq = Math.max(
                conversation.last_seq,
                message.seq,
            )
            conversation.updated_at = new Date().toISOString()
            conversation.unread_count = unreadCount
        }
        conversations.value = sortConversations(conversations.value)
        conversationSearchResults.value = sortConversations(
            conversationSearchResults.value,
        )
    }

    // applyFriendRelationshipChange 更新私聊会话的继续发送权限
    function applyFriendRelationshipChange(peerUserID: string, active: boolean) {
        for (const conversation of [
            ...conversations.value,
            ...conversationSearchResults.value,
        ]) {
            if (
                conversation.conversation_type === 1 &&
                conversation.peer_user_id === peerUserID
            ) {
                conversation.direct_friendship_active = active
            }
        }
    }

    // loadConversations 加载首个会话列表窗口
    async function loadConversations() {
        loadingConversations.value = true
        try {
            const previousActive = conversations.value.find(
                (item) => item.id === activeConversationId.value,
            )
            const page = await conversationApi.listConversations(auth.token)
            let next = [...page.conversations]
            /*
             * 活跃会话可能不在最新一页。重连刷新首屏时保留它，
             * 避免用户正在阅读的旧会话因重新分页被关闭。
             */
            if (
                activeConversationId.value &&
                previousActive &&
                !next.some((item) => item.id === activeConversationId.value)
            ) {
                next.push(previousActive)
            }
            conversations.value = sortConversations(next)
            conversationNextCursor.value = page.next_cursor
            hasMoreConversations.value = page.has_more
            if (!activeConversationId.value && conversations.value.length > 0) {
                await selectConversation(conversations.value[0].id)
            }
        } catch (cause) {
            setError(cause instanceof Error ? cause.message : '加载会话失败')
        } finally {
            loadingConversations.value = false
        }
    }

    // loadMoreConversations 使用复合游标加载更多会话
    async function loadMoreConversations() {
        if (conversationSearchQuery.value) {
            await loadMoreSearchResults()
            return
        }
        if (
            loadingMoreConversations.value ||
            !hasMoreConversations.value ||
            !conversationNextCursor.value
        ) {
            return
        }
        loadingMoreConversations.value = true
        try {
            const page = await conversationApi.listConversations(
                auth.token,
                {cursor: conversationNextCursor.value},
            )
            const merged = new Map<string, Conversation>()
            for (const conversation of [
                ...conversations.value,
                ...page.conversations,
            ]) {
                merged.set(conversation.id, conversation)
            }
            conversations.value = sortConversations([...merged.values()])
            conversationNextCursor.value = page.next_cursor
            hasMoreConversations.value = page.has_more
        } catch (cause) {
            setError(
                cause instanceof Error ? cause.message : '加载更多会话失败',
            )
        } finally {
            loadingMoreConversations.value = false
        }
    }

    // loadMoreSearchResults 加载会话搜索结果的下一页
    async function loadMoreSearchResults() {
        const query = conversationSearchQuery.value
        if (
            loadingMoreConversations.value ||
            !hasMoreSearchResults.value ||
            !searchNextCursor.value
        ) {
            return
        }
        loadingMoreConversations.value = true
        try {
            const page = await conversationApi.listConversations(auth.token, {
                cursor: searchNextCursor.value,
                query,
            })
            if (conversationSearchQuery.value !== query) return

            const merged = new Map<string, Conversation>()
            for (const conversation of [
                ...conversationSearchResults.value,
                ...page.conversations,
            ]) {
                merged.set(conversation.id, conversation)
            }
            conversationSearchResults.value = sortConversations([
                ...merged.values(),
            ])
            searchNextCursor.value = page.next_cursor
            hasMoreSearchResults.value = page.has_more
        } catch (cause) {
            setError(
                cause instanceof Error ? cause.message : '加载更多搜索结果失败',
            )
        } finally {
            loadingMoreConversations.value = false
        }
    }

    // searchConversations 按标题搜索会话
    async function searchConversations(rawQuery: string) {
        const query = rawQuery.trim()
        const generation = ++searchGeneration
        conversationSearchQuery.value = query
        conversationSearchResults.value = []
        searchNextCursor.value = ''
        hasMoreSearchResults.value = false
        if (!query) {
            searchingConversations.value = false
            return
        }

        searchingConversations.value = true
        clearError()
        try {
            const page = await conversationApi.listConversations(auth.token, {
                query,
            })
            if (
                generation !== searchGeneration ||
                conversationSearchQuery.value !== query
            ) {
                return
            }
            conversationSearchResults.value = sortConversations(
                page.conversations,
            )
            searchNextCursor.value = page.next_cursor
            hasMoreSearchResults.value = page.has_more
        } catch (cause) {
            if (generation === searchGeneration) {
                setError(
                    cause instanceof Error ? cause.message : '搜索会话失败',
                )
            }
        } finally {
            if (generation === searchGeneration) {
                searchingConversations.value = false
            }
        }
    }

    // clearConversationSearch 清空会话搜索并恢复普通列表
    function clearConversationSearch() {
        searchGeneration += 1
        conversationSearchQuery.value = ''
        conversationSearchResults.value = []
        searchNextCursor.value = ''
        hasMoreSearchResults.value = false
        searchingConversations.value = false
    }

    // refreshConversation 刷新单个会话摘要和最新消息窗口
    async function refreshConversation(conversationId: string) {
        try {
            const conversation = await conversationApi.getConversation(
                auth.token,
                conversationId,
            )
            upsertConversation(conversation)
        } catch {
            // 会话摘要刷新失败时，WebSocket 后续事件和定时对账仍可恢复。
        }
    }

    // setConversationPinned 更新会话个人置顶状态
    async function setConversationPinned(
        conversationId: string,
        pinned: boolean,
    ) {
        clearError()
        try {
            await conversationApi.setConversationPinned(
                auth.token,
                conversationId,
                pinned,
            )
            /*
             * 置顶状态会改变排序分组，旧游标不再代表稳定边界。
             * 重新加载首屏可同时刷新状态和游标，避免后续翻页重复或漏项。
             */
            if (conversationSearchQuery.value) {
                await searchConversations(conversationSearchQuery.value)
            } else {
                await loadConversations()
            }
        } catch (cause) {
            setError(
                cause instanceof Error ? cause.message : '更新会话置顶失败',
            )
        }
    }

    // createDirect 创建私聊并切换到新会话
    async function createDirect(peerUserId: string) {
        clearError()
        const conversation = await conversationApi.createDirect(
            auth.token,
            peerUserId.trim(),
        )
        upsertConversation(conversation)
        await selectConversation(conversation.id)
    }

    // createGroup 创建群聊并写入会话列表
    async function createGroup(
        name: string,
        memberUserIds: string[],
        inviteRemark: string,
    ) {
        clearError()
        const conversation = await conversationApi.createGroup(
            auth.token,
            name.trim(),
            memberUserIds,
            inviteRemark.trim(),
        )
        upsertConversation(conversation)
        await selectConversation(conversation.id)
    }

    // selectConversation 切换当前会话并加载成员和最新消息
    async function selectConversation(conversationId: string) {
        const searchMatch = conversationSearchResults.value.find(
            (item) => item.id === conversationId,
        )
        if (searchMatch) {
            conversations.value = upsertConversationInto(
                conversations.value,
                searchMatch,
            )
        }
        if (conversationSearchQuery.value) {
            clearConversationSearch()
        }
        activeConversationId.value = conversationId
        readStates.value = []
        const conversation = conversations.value.find(
            (item) => item.id === conversationId,
        )
        const tasks: Promise<void>[] = []
        if (
            conversation?.conversation_type === 2 &&
            !membersByConversation.value[conversationId]
        ) {
            tasks.push(loadMembers(conversationId))
        }
        await Promise.all(tasks)
        if (conversation) {
            await ensureConversationLatestWindow(conversation)
        }
        await markRead()
        await refreshReadStates()
        await refreshPresence()
    }

    // loadMembers 加载当前会话成员及角色
    async function loadMembers(conversationId: string, force = false) {
        if (!force && membersByConversation.value[conversationId]) {
            return
        }
        loadingMembers.value = true
        try {
            membersByConversation.value = {
                ...membersByConversation.value,
                [conversationId]: await conversationApi.listMembers(
                    auth.token,
                    conversationId,
                ),
            }
            await refreshPresence()
        } catch (cause) {
            setError(cause instanceof Error ? cause.message : '加载群成员失败')
        } finally {
            loadingMembers.value = false
        }
    }

    // inviteGroupMembers 邀请好友加入当前群聊
    async function inviteGroupMembers(
        memberUserIds: string[],
        inviteRemark: string,
    ): Promise<number> {
        const conversationId = activeConversationId.value
        const conversation = activeConversation.value
        if (!conversationId || conversation?.conversation_type !== 2) return 0
        clearError()
        const invitedCount = await conversationApi.inviteGroupMembers(
            auth.token,
            conversationId,
            memberUserIds,
            inviteRemark.trim(),
        )
        await loadMembers(conversationId, true)
        return invitedCount
    }

    // inviteUserToGroup 向指定群聊邀请一位好友
    async function inviteUserToGroup(
        conversationId: string,
        userId: string,
        inviteRemark: string,
    ) {
        clearError()
        await conversationApi.inviteGroupMembers(
            auth.token,
            conversationId,
            [userId],
            inviteRemark.trim(),
        )
    }

    // removeGroupMember 从当前群聊移除指定成员
    async function removeGroupMember(targetUserID: string) {
        const conversationId = activeConversationId.value
        const conversation = activeConversation.value
        if (!conversationId || conversation?.conversation_type !== 2) return
        clearError()
        await conversationApi.removeGroupMember(
            auth.token,
            conversationId,
            targetUserID,
        )
        if (targetUserID === auth.user?.id) {
            await removeConversationFromState(conversationId)
            return
        }
        await loadMembers(conversationId, true)
        conversation.member_count = (
            membersByConversation.value[conversationId] ?? []
        ).length
    }

    // setGroupMemberRole 调整当前群成员角色
    async function setGroupMemberRole(
        targetUserID: string,
        role: 1 | 2,
    ) {
        const conversationId = activeConversationId.value
        const conversation = activeConversation.value
        if (!conversationId || conversation?.conversation_type !== 2) return
        clearError()
        await conversationApi.setGroupMemberRole(
            auth.token,
            conversationId,
            targetUserID,
            role,
        )
        await loadMembers(conversationId, true)
    }

    // transferGroupOwner 转让当前群主身份
    async function transferGroupOwner(targetUserID: string) {
        const conversationId = activeConversationId.value
        const conversation = activeConversation.value
        if (!conversationId || conversation?.conversation_type !== 2) return
        clearError()
        await conversationApi.transferGroupOwner(
            auth.token,
            conversationId,
            targetUserID,
        )
        await loadMembers(conversationId, true)
    }

    // dissolveActiveGroup 解散当前群聊并清理本地状态
    async function dissolveActiveGroup() {
        const conversationId = activeConversationId.value
        const conversation = activeConversation.value
        if (!conversationId || conversation?.conversation_type !== 2) return
        clearError()
        await conversationApi.dissolveGroup(auth.token, conversationId)
        await removeConversationFromState(conversationId)
    }

    // updateGroupProfile 更新当前群资料
    async function updateGroupProfile(name: string, avatarUrl: string) {
        const conversationId = activeConversationId.value
        const conversation = activeConversation.value
        if (!conversationId || conversation?.conversation_type !== 2) return
        clearError()
        const response = await conversationApi.updateGroupProfile(
            auth.token,
            conversationId,
            name,
            avatarUrl,
        )
        conversation.name = response.name
        conversation.title = response.name
        conversation.avatar_url = response.avatar_url
    }

    // removeConversationFromState 从本地会话列表移除不可访问会话
    async function removeConversationFromState(conversationId: string) {
        conversations.value = conversations.value.filter(
            (item) => item.id !== conversationId,
        )
        delete messagesByConversation.value[conversationId]
        delete membersByConversation.value[conversationId]
        clearSequenceState(conversationId)
        activeConversationId.value = ''
        readStates.value = []
        if (conversations.value.length > 0) {
            await selectConversation(conversations.value[0].id)
        }
    }

    // resyncConversationLatestWindow 重载会话最新窗口并重建序号基线
    async function resyncConversationLatestWindow(conversationId: string) {
        const state = sequenceStateFor(conversationId)
        const generation = state.generation + 1
        state.generation = generation
        state.resyncing = true
        if (state.retryTimer) {
            window.clearTimeout(state.retryTimer)
            state.retryTimer = undefined
        }
        state.repairing = false
        state.repairRequested = false
        state.repairAttempts = 0
        loadingMessages.value = true
        clearError()
        try {
            const page = await messageApi.listMessages(auth.token, conversationId, {
                limit: 40,
            })
            // 会话切换或新一轮重同步会递增代次；较早发出的请求不得覆盖当前窗口。
            if (
                sequenceStates.get(conversationId) !== state ||
                state.generation !== generation
            ) {
                return
            }

            const latestMessages = [...page.messages].sort(
                (left, right) => left.seq - right.seq,
            )
            const latestSeq = latestMessages.at(-1)?.seq ?? 0
            const conversationSummary = conversations.value.find(
                (item) => item.id === conversationId,
            )
            if (conversationSummary) {
                conversationSummary.last_seq = Math.max(
                    conversationSummary.last_seq,
                    latestSeq,
                )
            }

            /*
             * 最新页是新的历史窗口。旧窗口可能距离最新消息几千条，
             * 不能直接拼接，否则向前分页会从错误的 before_seq 开始。
             * 请求期间新到的实时消息比最新页更新，必须继续缓冲，避免
             * HTTP 响应覆盖 WebSocket 已经送达的消息。
             */

            const buffered = new Map<string, ChatMessage>()
            for (const message of [
                ...(messagesByConversation.value[conversationId] ?? []),
                ...state.pendingMessages.values(),
            ]) {
                if (message.seq > latestSeq) {
                    buffered.set(message.message_id, message)
                }
            }
            messagesByConversation.value = {
                ...messagesByConversation.value,
                [conversationId]: latestMessages,
            }
            hasMoreByConversation.value = {
                ...hasMoreByConversation.value,
                [conversationId]: page.has_more,
            }

            state.cursor = createSequenceCursor(latestSeq)
            state.pendingMessages = new Map()
            for (const message of [...buffered.values()].sort(
                (left, right) => left.seq - right.seq,
            )) {
                observeSequence(state.cursor, message.seq)
                if (message.seq <= state.cursor.contiguousSeq) {
                    storeMessage(conversationId, message)
                } else {
                    state.pendingMessages.set(message.seq, message)
                }
            }

            if (state.cursor.pendingSeqs.size > 0) {
                scheduleGapRepair(conversationId)
            }
        } catch (cause) {
            setError(cause instanceof Error ? cause.message : '同步最新消息失败')
        } finally {
            if (
                sequenceStates.get(conversationId) === state &&
                state.generation === generation
            ) {
                state.resyncing = false
                if (state.cursor.pendingSeqs.size > 0) {
                    scheduleGapRepair(conversationId)
                }
            }
            loadingMessages.value = false
        }
    }

    // ensureConversationLatestWindow 确保当前会话具备可用最新消息窗口
    async function ensureConversationLatestWindow(
        conversation: Conversation,
    ) {
        const state = sequenceStateFor(conversation.id)
        const hasWindow =
            (messagesByConversation.value[conversation.id]?.length ?? 0) > 0
        if (
            needsLatestWindowSync(
                hasWindow,
                conversation.last_seq,
                state.cursor.contiguousSeq,
            )
        ) {
            await resyncConversationLatestWindow(conversation.id)
            return
        }
        if (state.cursor.pendingSeqs.size > 0) {
            await repairConversationGap(conversation.id)
        }
    }

    // loadOlderMessages 按 before_seq 加载更早历史消息
    async function loadOlderMessages() {
        const conversationId = activeConversationId.value
        const messages = messagesByConversation.value[conversationId] ?? []
        if (!conversationId || messages.length === 0) return
        if (!hasMoreByConversation.value[conversationId]) return

        const page = await messageApi.listMessages(auth.token, conversationId, {
            beforeSeq: messages[0].seq,
            limit: 30,
        })
        const merged = new Map<string, ChatMessage>()
        for (const message of [...page.messages, ...messages]) {
            merged.set(message.message_id, message)
        }
        messagesByConversation.value = {
            ...messagesByConversation.value,
            [conversationId]: [...merged.values()].sort(
                (left, right) => left.seq - right.seq,
            ),
        }
        hasMoreByConversation.value = {
            ...hasMoreByConversation.value,
            [conversationId]: page.has_more,
        }
    }

    // sendMessage 发送文本消息并处理 ACK、缺口和摘要刷新
    async function sendMessage(content: string) {
        const conversationId = activeConversationId.value
        const trimmed = content.trim()
        if (!conversationId || !trimmed) return

        clearError()
        const response = await messageApi.sendText(
            auth.token,
            conversationId,
            crypto.randomUUID(),
            trimmed,
        )
        const observation = acceptMessageSequence(conversationId, {
            message_id: response.message_id,
            conversation_id: response.conversation_id,
            sender_id: auth.user?.id ?? '',
            seq: response.seq,
            message_type: 1,
            content: trimmed,
            created_at: response.created_at,
            recalled: false,
            outgoing: true,
        })
        const conversation = conversations.value.find((item) => item.id === conversationId)
        if (conversation) {
            conversation.last_seq = Math.max(conversation.last_seq, response.seq)
            conversation.updated_at = response.created_at
            if (!observation.gapDetected) {
                conversation.last_read_seq = Math.max(
                    conversation.last_read_seq,
                    sequenceStateFor(conversationId).cursor.contiguousSeq,
                )
                conversation.unread_count = 0
            }
            conversations.value = sortConversations(conversations.value)
        }
        void refreshReadStates()
        window.setTimeout(() => {
            void refreshDelivery(conversationId, response.message_id)
        }, 1200)
    }

    // sendAttachment 上传附件并发送图片或文件消息
    async function sendAttachment(file: File) {
        const conversationId = activeConversationId.value
        if (!conversationId) return

        clearError()
        uploadingAttachment.value = true
        uploadProgress.value = 0
        try {
            const uploaded = await messageApi.uploadAttachment(
                auth.token,
                file,
                (progress) => {
                    uploadProgress.value = progress
                },
            )
            const response = await messageApi.sendAttachmentMessage(
                auth.token,
                conversationId,
                crypto.randomUUID(),
                uploaded.message_type,
                uploaded.attachment_id,
            )
            const observation = acceptMessageSequence(conversationId, {
                message_id: response.message_id,
                conversation_id: response.conversation_id,
                sender_id: auth.user?.id ?? '',
                seq: response.seq,
                message_type: uploaded.message_type,
                content: '',
                attachment_id: uploaded.attachment_id,
                attachment_url: uploaded.url,
                attachment_name: uploaded.filename,
                created_at: response.created_at,
                recalled: false,
                outgoing: true,
            })
            const conversation = conversations.value.find(
                (item) => item.id === conversationId,
            )
            if (conversation) {
                conversation.last_seq = Math.max(conversation.last_seq, response.seq)
                conversation.updated_at = response.created_at
                if (!observation.gapDetected) {
                    conversation.last_read_seq = Math.max(
                        conversation.last_read_seq,
                        sequenceStateFor(conversationId).cursor.contiguousSeq,
                    )
                    conversation.unread_count = 0
                }
                conversations.value = sortConversations(conversations.value)
            }
            void refreshReadStates()
            window.setTimeout(() => {
                void refreshDelivery(conversationId, response.message_id)
            }, 1200)
        } catch (cause) {
            setError(cause instanceof Error ? cause.message : '发送附件失败')
            throw cause
        } finally {
            uploadingAttachment.value = false
            uploadProgress.value = 0
        }
    }

    // refreshDelivery 刷新消息送达统计
    async function refreshDelivery(conversationId: string, messageId: string) {
        try {
            const delivery = await messageApi.getDeliverySummary(
                auth.token,
                conversationId,
                messageId,
            )
            const messages = messagesByConversation.value[conversationId] ?? []
            const message = messages.find((item) => item.message_id === messageId)
            if (message) {
                message.delivery = delivery
            }
        } catch {
            // 送达状态刷新失败不回滚已经成功发送的消息。
        }
    }

    // recallMessage 撤回消息并同步本地状态
    async function recallMessage(messageId: string) {
        const conversationId = activeConversationId.value
        if (!conversationId) return
        clearError()
        await messageApi.recallMessage(auth.token, conversationId, messageId)
        markMessageRecalled(
            conversationId,
            messageId,
            auth.user?.id,
        )
    }

    // resolveRealtimeAttachment 把实时附件路径换取为短时下载地址
    async function resolveRealtimeAttachment(
        conversationId: string,
        messageId: string,
        attachmentURL?: string,
    ) {
        if (!attachmentURL?.startsWith('/api/v1/attachments/')) return
        const attachmentId = attachmentURL.split('/')[4]
        if (!attachmentId) return
        try {
            const info = await messageApi.getAttachmentInfo(auth.token, attachmentId)
            const message = (
                messagesByConversation.value[conversationId] ?? []
            ).find((item) => item.message_id === messageId)
            if (message) {
                message.attachment_id = info.attachment_id
                message.attachment_url = info.url
                message.attachment_name = info.filename
            }
        } catch {
            // 签名地址解析失败时保留附件消息，用户可稍后重新进入会话。
        }
    }

    // refreshReadStates 刷新当前会话成员已读游标
    async function refreshReadStates() {
        const conversationId = activeConversationId.value
        if (!conversationId) return
        try {
            readStates.value = await messageApi.listReadStates(auth.token, conversationId)
        } catch {
            readStates.value = []
        }
    }

    // refreshPresence 刷新当前成员在线状态
    async function refreshPresence() {
        const conversation = activeConversation.value
        if (!conversation) {
            presenceByUser.value = {}
            return
        }
        let userIds: string[] = []
        if (conversation.conversation_type === 2) {
            userIds = (
                membersByConversation.value[conversation.id] ?? []
            ).map((member) => member.user_id)
        } else if (conversation.peer_user_id) {
            userIds = [conversation.peer_user_id]
        }
        userIds = userIds.filter((userID) => userID !== auth.user?.id)
        try {
            presenceByUser.value = await presenceApi.queryPresence(
                auth.token,
                userIds,
            )
        } catch {
            // 在线状态是辅助信息，查询失败不阻塞聊天主链路。
            presenceByUser.value = {}
        }
    }

    // sendSocket 通过 WebSocket 发送 ACK 或已读信令
    function sendSocket(payload: WsMessage) {
        return realtime.send(payload)
    }

    // sendAck 提交消息设备送达回执
    function sendAck(messageId: string) {
        sendSocket({
            action: 'ack',
            message_id: messageId,
        })
    }

    // markRead 单调推进当前会话已读位置
    async function markRead() {
        const conversation = activeConversation.value
        if (!conversation) return
        const state = sequenceStateFor(conversation.id)
        if (state.cursor.pendingSeqs.size > 0) return
        const seq = state.cursor.contiguousSeq
        if (!seq || seq <= conversation.last_read_seq) return
        if (
            sendSocket({
                action: 'read',
                message_id: '',
                conversation_id: conversation.id,
                seq,
            })
        ) {
            conversation.last_read_seq = Math.max(conversation.last_read_seq, seq)
            conversation.unread_count = 0
            window.setTimeout(() => {
                void refreshReadStates()
            }, 300)
        }
    }

    // handleSocketMessage 根据动作类型处理实时消息、撤回和群邀请
    function handleSocketMessage(payload: WsMessage) {
        if (
            payload.action === 'conversation_created' &&
            payload.conversation_id
        ) {
            void refreshConversation(payload.conversation_id)
            return
        }
        if (
            payload.action === 'friend_relationship_changed' &&
            payload.peer_user_id
        ) {
            const active = Boolean(payload.relationship_active)
            applyFriendRelationshipChange(payload.peer_user_id, active)
            friendships.applyRelationshipChange(
                payload.peer_user_id,
                active,
                payload.friend_visible ?? true,
            )
            return
        }
        if (
            payload.action === 'group_invitation' ||
            payload.action === 'group_invitation_cancelled'
        ) {
            void groupInvitations.loadReceived()
            return
        }
        if (
            payload.action === 'message_recalled' &&
            payload.message_id &&
            payload.conversation_id
        ) {
            markMessageRecalled(
                payload.conversation_id,
                payload.message_id,
                payload.recalled_by,
                payload.recalled_at,
            )
            return
        }
        if (
            payload.action !== 'message' ||
            !payload.message_id ||
            !payload.conversation_id ||
            !payload.sender_id ||
            !payload.seq
        ) {
            return
        }
        const message: ChatMessage = {
            message_id: payload.message_id,
            conversation_id: payload.conversation_id,
            sender_id: payload.sender_id,
            seq: payload.seq,
            message_type: payload.msg_type ?? 1,
            content: payload.content ?? '',
            attachment_id: undefined,
            // 实时事件只携带受保护的内容路径，浏览器 img 无法附加 JWT。
            // 先显示加载态，换取短时签名 URL 后再交给图片或下载组件。
            attachment_url: payload.url?.startsWith('/api/v1/attachments/')
                ? undefined
                : payload.url,
            attachment_name: payload.file_name,
            realtime_attachment_path: payload.url,
            recalled: false,
            created_at: new Date().toISOString(),
        }
        const observation = acceptMessageSequence(
            payload.conversation_id,
            message,
        )
        updateConversationFromMessage(message)
        sendAck(message.message_id)
        if (
            payload.conversation_id === activeConversationId.value &&
            document.visibilityState === 'visible' &&
            !observation.gapDetected
        ) {
            void markRead()
        }
    }

    // handleSocketConnected 连接恢复后重建最新窗口和会话摘要
    async function handleSocketConnected() {
        const activeConversationBeforeReconnect = activeConversationId.value
        await loadConversations()
        const active = conversations.value.find(
            (conversation) => conversation.id === activeConversationId.value,
        )
        if (activeConversationBeforeReconnect && active) {
            await ensureConversationLatestWindow(active)
        }
        await markRead()
    }

    // reconcileActiveConversation 定时对账当前会话最新窗口
    async function reconcileActiveConversation() {
        const conversationId = activeConversationId.value
        if (!conversationId) return
        try {
            const conversation = await conversationApi.getConversation(
                auth.token,
                conversationId,
            )
            upsertConversation(conversation)

            const state = sequenceStateFor(conversationId)
            const hasWindow =
                (messagesByConversation.value[conversationId]?.length ?? 0) > 0
            if (!hasWindow || state.cursor.contiguousSeq === 0) {
                await ensureConversationLatestWindow(conversation)
                return
            }

            /*
             * 活跃会话已经建立连续实时窗口。对账只补服务端最后序号
             * 与本地连续水位之间的区间，避免最后一条推送丢失后因没有
             * 后续消息而永远无法收敛。历史加载仍由客户端主动向前翻页。
             */

            if (
                conversation.last_seq > state.cursor.contiguousSeq ||
                state.cursor.pendingSeqs.size > 0
            ) {
                await repairConversationGap(conversationId)
            }
        } catch {
            // 对账是可靠性兜底；页面下一次定时触发时会再次尝试。
        }
    }

    // connectWebSocket 为当前登录用户建立实时连接
    function connectWebSocket() {
        return realtime.connect()
    }

    // disconnectWebSocket 断开实时连接并清理定时任务
    function disconnectWebSocket() {
        realtime.disconnect()
    }

    // reset 重置聊天 Store 的会话、消息和连接状态
    function reset() {
        disconnectWebSocket()
        for (const conversationId of [...sequenceStates.keys()]) {
            clearSequenceState(conversationId)
        }
        conversations.value = []
        messagesByConversation.value = {}
        membersByConversation.value = {}
        activeConversationId.value = ''
        readStates.value = []
        presenceByUser.value = {}
        conversationNextCursor.value = ''
        hasMoreConversations.value = false
        clearConversationSearch()
        error.value = ''
    }

    return {
        conversations,
        displayedConversations,
        messagesByConversation,
        membersByConversation,
        activeConversationId,
        activeConversation,
        activeMessages,
        activeMembers,
        activeReadStates,
        presenceByUser,
        connectionState,
        loadingConversations,
        loadingMoreConversations,
        hasMoreConversations,
        hasMoreDisplayedConversations,
        conversationSearchQuery,
        conversationSearchResults,
        searchingConversations,
        loadingMessages,
        loadingMembers,
        uploadingAttachment,
        uploadProgress,
        hasMoreByConversation,
        error,
        conversationTitle,
        loadConversations,
        loadMoreConversations,
        searchConversations,
        clearConversationSearch,
        setConversationPinned,
        createDirect,
        createGroup,
        selectConversation,
        loadMembers,
        inviteGroupMembers,
        inviteUserToGroup,
        removeGroupMember,
        setGroupMemberRole,
        transferGroupOwner,
        dissolveActiveGroup,
        updateGroupProfile,
        loadOlderMessages,
        sendMessage,
        sendAttachment,
        recallMessage,
        connectWebSocket,
        disconnectWebSocket,
        markRead,
        refreshPresence,
        reconcileActiveConversation,
        applyFriendRelationshipChange,
        reportError: setError,
        clearError,
        reset,
    }
})
