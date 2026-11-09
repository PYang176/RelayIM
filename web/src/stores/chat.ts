import {defineStore} from 'pinia'
import {computed, ref, shallowRef} from 'vue'

import * as conversationApi from '@/api/conversations'
import * as messageApi from '@/api/messages'
import * as presenceApi from '@/api/presence'
import {useChatSocket} from '@/composables/useChatSocket'
import {useAuthStore} from '@/stores/auth'
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
    const conversations = ref<Conversation[]>([])
    const messagesByConversation = ref<Record<string, ChatMessage[]>>({})
    const membersByConversation = ref<Record<string, ConversationMember[]>>({})
    const activeConversationId = shallowRef('')
    const readStates = ref<ReadState[]>([])
    const presenceByUser = ref<Record<string, boolean>>({})
    const loadingConversations = shallowRef(false)
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

    const realtime = useChatSocket({
        getToken: () => auth.token,
        onConnected: handleSocketConnected,
        onMessage: handleSocketMessage,
        onError: setError,
    })
    const connectionState = realtime.connectionState

    function setError(message: string) {
        error.value = message
    }

    function clearError() {
        error.value = ''
    }

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

    function clearSequenceState(conversationId: string) {
        const state = sequenceStates.get(conversationId)
        if (state?.retryTimer) {
            window.clearTimeout(state.retryTimer)
        }
        sequenceStates.delete(conversationId)
    }

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

    function conversationTitle(conversation: Conversation): string {
        return conversation.title || conversation.name || '私聊'
    }

    function upsertConversation(conversation: Conversation) {
        const index = conversations.value.findIndex((item) => item.id === conversation.id)
        if (index >= 0) {
            conversations.value[index] = conversation
            return
        }
        conversations.value.unshift(conversation)
    }

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

    function updateConversationFromMessage(message: Message) {
        const conversation = conversations.value.find(
            (item) => item.id === message.conversation_id,
        )
        if (!conversation) {
            void loadConversations()
            return
        }
        conversation.last_seq = Math.max(conversation.last_seq, message.seq)
        const shouldIncrementUnread =
            message.sender_id !== auth.user?.id &&
            (
                message.conversation_id !== activeConversationId.value ||
                document.visibilityState !== 'visible'
            )
        if (shouldIncrementUnread) {
            conversation.unread_count += 1
        }
    }

    async function loadConversations() {
        loadingConversations.value = true
        try {
            conversations.value = await conversationApi.listConversations(auth.token)
            if (!activeConversationId.value && conversations.value.length > 0) {
                await selectConversation(conversations.value[0].id)
            }
        } catch (cause) {
            setError(cause instanceof Error ? cause.message : '加载会话失败')
        } finally {
            loadingConversations.value = false
        }
    }

    async function createDirect(peerUserId: string) {
        clearError()
        const conversation = await conversationApi.createDirect(
            auth.token,
            peerUserId.trim(),
        )
        upsertConversation(conversation)
        await selectConversation(conversation.id)
    }

    async function createGroup(name: string, memberUserIds: string[]) {
        clearError()
        const conversation = await conversationApi.createGroup(
            auth.token,
            name.trim(),
            memberUserIds,
        )
        upsertConversation(conversation)
        await selectConversation(conversation.id)
    }

    async function selectConversation(conversationId: string) {
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

    async function inviteGroupMembers(
        memberUserIds: string[],
    ): Promise<number> {
        const conversationId = activeConversationId.value
        const conversation = activeConversation.value
        if (!conversationId || conversation?.conversation_type !== 2) return 0
        clearError()
        const invitedCount = await conversationApi.inviteGroupMembers(
            auth.token,
            conversationId,
            memberUserIds,
        )
        await loadMembers(conversationId, true)
        return invitedCount
    }

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

    async function dissolveActiveGroup() {
        const conversationId = activeConversationId.value
        const conversation = activeConversation.value
        if (!conversationId || conversation?.conversation_type !== 2) return
        clearError()
        await conversationApi.dissolveGroup(auth.token, conversationId)
        await removeConversationFromState(conversationId)
    }

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
            if (!observation.gapDetected) {
                conversation.last_read_seq = Math.max(
                    conversation.last_read_seq,
                    sequenceStateFor(conversationId).cursor.contiguousSeq,
                )
                conversation.unread_count = 0
            }
        }
        void refreshReadStates()
        window.setTimeout(() => {
            void refreshDelivery(conversationId, response.message_id)
        }, 1200)
    }

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
                if (!observation.gapDetected) {
                    conversation.last_read_seq = Math.max(
                        conversation.last_read_seq,
                        sequenceStateFor(conversationId).cursor.contiguousSeq,
                    )
                    conversation.unread_count = 0
                }
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

    async function refreshReadStates() {
        const conversationId = activeConversationId.value
        if (!conversationId) return
        try {
            readStates.value = await messageApi.listReadStates(auth.token, conversationId)
        } catch {
            readStates.value = []
        }
    }

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

    function sendSocket(payload: WsMessage) {
        return realtime.send(payload)
    }

    function sendAck(messageId: string) {
        sendSocket({
            action: 'ack',
            message_id: messageId,
        })
    }

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

    function handleSocketMessage(payload: WsMessage) {
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

    async function reconcileActiveConversation() {
        const conversationId = activeConversationId.value
        if (!conversationId) return
        try {
            const latestConversations = await conversationApi.listConversations(
                auth.token,
            )
            const conversation = latestConversations.find(
                (item) => item.id === conversationId,
            )
            if (!conversation) return
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

    function connectWebSocket() {
        return realtime.connect()
    }

    function disconnectWebSocket() {
        realtime.disconnect()
    }

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
        error.value = ''
    }

    return {
        conversations,
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
        loadingMessages,
        loadingMembers,
        uploadingAttachment,
        uploadProgress,
        hasMoreByConversation,
        error,
        conversationTitle,
        loadConversations,
        createDirect,
        createGroup,
        selectConversation,
        loadMembers,
        inviteGroupMembers,
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
        reportError: setError,
        clearError,
        reset,
    }
})
