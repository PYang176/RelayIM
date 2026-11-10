<script setup lang="ts">
import {computed, shallowRef, watch} from 'vue'

import {getUserProfile} from '@/api/auth'
import {getRelationshipStatus} from '@/api/friends'
import AvatarPreviewDialog from '@/components/AvatarPreviewDialog.vue'
import FriendRequestForm from '@/components/FriendRequestForm.vue'
import {useAuthStore} from '@/stores/auth'
import {useFriendshipStore} from '@/stores/friendships'
import type {
    Conversation,
    Friend,
    FriendRelationshipStatus,
    UserSummary,
} from '@/types/api'

const props = defineProps<{
    friend: Friend | null
    groups: Conversation[]
}>()

const emit = defineEmits<{
    message: [userID: string]
    addToGroup: [friend: Friend]
    friendRemoved: [userID: string]
}>()

const auth = useAuthStore()
const friendships = useFriendshipStore()
const profile = shallowRef<UserSummary | null>(null)
const loading = shallowRef(false)
const profileError = shallowRef('')
const relationship = shallowRef<FriendRelationshipStatus | null>(null)
const relationshipLoading = shallowRef(false)
const relationshipError = shallowRef('')
const requestFormOpen = shallowRef(false)
const avatarPreviewOpen = shallowRef(false)
const editingRemark = shallowRef(false)
const remarkDraft = shallowRef('')
const confirmingBlock = shallowRef(false)
const confirmingDelete = shallowRef(false)
let requestGeneration = 0
let relationshipGeneration = 0

const displayName = computed(
    () =>
        props.friend?.remark ||
        profile.value?.nickname ||
        props.friend?.user.nickname ||
        props.friend?.user.username ||
        '',
)
const avatarUrl = computed(
    () => profile.value?.avatar_url || props.friend?.user.avatar_url || '',
)
const avatarLetter = computed(() => displayName.value.slice(0, 1))
const relationshipActive = computed(
    () => Boolean(props.friend?.relationship_active),
)
const canStartChat = computed(
    () =>
        Boolean(props.friend) &&
        (relationship.value?.can_message ??
            (relationshipActive.value && !props.friend?.blocked_by_me)),
)
const canAddToGroup = computed(
    () => canStartChat.value && props.groups.length > 0,
)
const canRequest = computed(() => Boolean(relationship.value?.can_request))
const canEditRemark = computed(() => canStartChat.value)
const relationshipLabel = computed(() => {
    if (!props.friend) return ''
    if (props.friend.blocked_by_me) return '已拉黑'
    return relationshipActive.value ? '好友关系正常' : '对方已删除你'
})
const relationshipNotice = computed(() => {
    if (!props.friend) return ''
    if (relationshipLoading.value) return '正在确认好友关系…'
    if (relationshipError.value) return relationshipError.value
    if (props.friend.blocked_by_me) {
        return '你已拉黑该联系人，取消拉黑后才能继续私聊或邀请入群。'
    }
    if (relationship.value?.has_pending_request) {
        return relationship.value.requested_by_me
            ? '好友申请已发送，等待对方处理。'
            : '对方已发送好友申请，请前往联系人申请中处理。'
    }
    if (!relationshipActive.value) {
        return '双方当前不是好友，可以重新申请添加。'
    }
    return ''
})

watch(
    () => props.friend?.user.id ?? '',
    async (userID) => {
        const generation = ++requestGeneration
        profile.value = null
        profileError.value = ''
        avatarPreviewOpen.value = false
        editingRemark.value = false
        remarkDraft.value = props.friend?.remark ?? ''
        confirmingBlock.value = false
        confirmingDelete.value = false
        requestFormOpen.value = false
        if (!userID) {
            loading.value = false
            return
        }
        loading.value = true
        try {
            const result = await getUserProfile(auth.token, userID)
            if (generation === requestGeneration) {
                profile.value = result
            }
        } catch (cause) {
            if (generation === requestGeneration) {
                profileError.value =
                    cause instanceof Error ? cause.message : '加载联系人资料失败'
            }
        } finally {
            if (generation === requestGeneration) {
                loading.value = false
            }
        }
    },
    {immediate: true},
)

watch(
    () => [
        props.friend?.user.id ?? '',
        Boolean(props.friend?.relationship_active),
        Boolean(props.friend?.blocked_by_me),
    ] as const,
    () => {
        void loadRelationshipStatus()
    },
    {immediate: true},
)

watch(
    () => props.friend?.remark ?? '',
    (remark) => {
        if (!editingRemark.value) {
            remarkDraft.value = remark
        }
    },
)

// openAvatar 打开当前联系人头像的大图预览
function openAvatar() {
    if (props.friend) {
        avatarPreviewOpen.value = true
    }
}

// startChat 从联系人资料进入私聊
function startChat() {
    if (props.friend && canStartChat.value) {
        emit('message', props.friend.user.id)
    }
}

// addToGroup 打开群聊邀请选择
function addToGroup() {
    if (props.friend && canAddToGroup.value) {
        emit('addToGroup', props.friend)
    }
}

// loadRelationshipStatus 读取当前联系人的好友申请和私聊权限
async function loadRelationshipStatus() {
    const userID = props.friend?.user.id ?? ''
    const generation = ++relationshipGeneration
    relationship.value = null
    relationshipError.value = ''
    requestFormOpen.value = false
    if (!userID) {
        relationshipLoading.value = false
        return
    }
    relationshipLoading.value = true
    try {
        const result = await getRelationshipStatus(auth.token, userID)
        if (generation === relationshipGeneration) {
            relationship.value = result
        }
    } catch (cause) {
        if (generation === relationshipGeneration) {
            relationshipError.value =
                cause instanceof Error
                    ? cause.message
                    : '加载好友关系状态失败'
        }
    } finally {
        if (generation === relationshipGeneration) {
            relationshipLoading.value = false
        }
    }
}

// openFriendRequestForm 展开重新添加好友的申请理由
function openFriendRequestForm() {
    if (!canRequest.value) return
    friendships.clearError()
    requestFormOpen.value = true
}

// submitFriendRequest 提交重新添加好友申请并刷新关系状态
async function submitFriendRequest(remark: string) {
    if (!props.friend || friendships.actionBusy) return
    if (await friendships.sendRequest(props.friend.user.id, remark)) {
        await loadRelationshipStatus()
    }
}

// startRemarkEdit 使用当前备注进入右侧就地编辑状态
function startRemarkEdit() {
    if (!props.friend || !canEditRemark.value) return
    friendships.clearError()
    remarkDraft.value = props.friend.remark
    editingRemark.value = true
}

// cancelRemarkEdit 放弃未保存的备注并恢复服务端值
function cancelRemarkEdit() {
    remarkDraft.value = props.friend?.remark ?? ''
    editingRemark.value = false
}

// saveRemark 保存好友备注，空内容表示清除备注
async function saveRemark() {
    if (!props.friend) return
    const updated = await friendships.updateRemark(
        props.friend.user.id,
        remarkDraft.value.trim(),
    )
    if (updated) {
        editingRemark.value = false
    }
}

// requestBlock 展开拉黑确认，避免资料页误操作
function requestBlock() {
    friendships.clearError()
    confirmingDelete.value = false
    confirmingBlock.value = true
}

// cancelBlockConfirmation 收起拉黑确认
function cancelBlockConfirmation() {
    confirmingBlock.value = false
}

// confirmBlock 确认拉黑当前联系人并刷新关系状态
async function confirmBlock() {
    if (!props.friend) return
    if (await friendships.blockUser(props.friend.user.id)) {
        confirmingBlock.value = false
    }
}

// unblockContact 取消拉黑当前联系人并恢复可私聊状态
async function unblockContact() {
    if (props.friend) {
        await friendships.unblockUser(props.friend.user.id)
    }
}

// requestDelete 展开删除确认，说明单向删除的结果
function requestDelete() {
    friendships.clearError()
    confirmingBlock.value = false
    confirmingDelete.value = true
}

// cancelDeleteConfirmation 收起删除确认
function cancelDeleteConfirmation() {
    confirmingDelete.value = false
}

// deleteContact 确认删除好友并通知页面清空当前选择
async function deleteContact() {
    if (!props.friend) return
    const userID = props.friend.user.id
    if (await friendships.removeFriend(userID)) {
        confirmingDelete.value = false
        emit('friendRemoved', userID)
    }
}
</script>

<template>
    <section class="contact-detail-panel">
        <div v-if="!friend" class="contact-placeholder">
            <span class="contact-placeholder-mark">联</span>
            <h1>选择一位联系人</h1>
            <p>左侧选择好友后，这里会显示头像、备注和个人资料。</p>
        </div>

        <article v-else class="contact-profile">
            <header class="contact-profile-header">
                <button
                        class="contact-profile-avatar"
                        :aria-label="`放大${displayName}的头像`"
                        type="button"
                        @click="openAvatar"
                >
                    <img
                            v-if="avatarUrl"
                            :alt="`${displayName}的头像`"
                            :src="avatarUrl"
                    />
                    <template v-else>{{ avatarLetter }}</template>
                </button>
                <div class="contact-profile-heading">
                    <h1>{{ displayName }}</h1>
                    <p>@{{ profile?.username || friend.user.username }}</p>
                    <span
                            :class="{
                                inactive:
                                    !relationshipActive || friend.blocked_by_me,
                            }"
                            class="contact-relationship-state"
                    >
                        {{ relationshipLabel }}
                    </span>
                </div>
            </header>

            <p v-if="loading" class="contact-profile-state" role="status">
                正在加载资料…
            </p>
            <p v-else-if="profileError" class="form-error" role="alert">
                {{ profileError }}
            </p>

            <dl class="contact-profile-fields">
                <div>
                    <dt>好友备注</dt>
                    <dd>
                        <form
                                v-if="editingRemark"
                                class="contact-remark-editor"
                                @submit.prevent="saveRemark"
                        >
                            <label class="sr-only" for="contact-friend-remark">
                                好友备注
                            </label>
                            <input
                                    id="contact-friend-remark"
                                    v-model="remarkDraft"
                                    :disabled="friendships.actionBusy"
                                    maxlength="64"
                                    placeholder="留空保存可删除备注"
                            />
                            <div class="contact-remark-editor-actions">
                                <button
                                        :disabled="friendships.actionBusy"
                                        class="primary-button compact"
                                        type="submit"
                                >
                                    {{ friendships.actionBusy ? '保存中…' : '保存' }}
                                </button>
                                <button
                                        :disabled="friendships.actionBusy"
                                        class="ghost-button"
                                        type="button"
                                        @click="cancelRemarkEdit"
                                >
                                    取消
                                </button>
                            </div>
                        </form>
                        <div v-else class="contact-remark-value">
                            <span>{{ friend.remark || '未设置备注' }}</span>
                            <button
                                    v-if="canEditRemark"
                                    class="text-button"
                                    type="button"
                                    @click="startRemarkEdit"
                            >
                                修改备注
                            </button>
                        </div>
                    </dd>
                </div>
                <div>
                    <dt>个人简介</dt>
                    <dd>{{ profile?.bio || '这个用户还没有填写个人简介。' }}</dd>
                </div>
            </dl>

            <section
                    class="contact-relationship-management"
                    aria-labelledby="contact-relationship-title"
            >
                <header>
                    <h2 id="contact-relationship-title">关系管理</h2>
                    <p>拉黑和删除会影响双方的私聊权限</p>
                </header>

                <div v-if="confirmingBlock" class="contact-confirmation" role="alert">
                    <p>
                        <strong>确认拉黑？</strong>
                        拉黑后双方不能私聊或发送好友申请，历史记录仍会保留。
                    </p>
                    <div>
                        <button
                                :disabled="friendships.actionBusy"
                                class="ghost-button danger-text"
                                type="button"
                                @click="confirmBlock"
                        >
                            {{ friendships.actionBusy ? '处理中…' : '确认拉黑' }}
                        </button>
                        <button
                                :disabled="friendships.actionBusy"
                                class="text-button"
                                type="button"
                                @click="cancelBlockConfirmation"
                        >
                            取消
                        </button>
                    </div>
                </div>

                <div
                        v-else-if="confirmingDelete"
                        class="contact-confirmation"
                        role="alert"
                >
                    <p>
                        <strong>确认删除好友？</strong>
                        删除后双方无法继续私聊，你的备注会被清除，对方仍会保留联系人。
                    </p>
                    <div>
                        <button
                                :disabled="friendships.actionBusy"
                                class="ghost-button danger-text"
                                type="button"
                                @click="deleteContact"
                        >
                            {{ friendships.actionBusy ? '处理中…' : '确认删除' }}
                        </button>
                        <button
                                :disabled="friendships.actionBusy"
                                class="text-button"
                                type="button"
                                @click="cancelDeleteConfirmation"
                        >
                            取消
                        </button>
                    </div>
                </div>

                <div v-else class="contact-relationship-buttons">
                    <button
                            v-if="friend.blocked_by_me"
                            :disabled="friendships.actionBusy"
                            class="ghost-button"
                            type="button"
                            @click="unblockContact"
                    >
                        取消拉黑
                    </button>
                    <button
                            v-else
                            :disabled="friendships.actionBusy"
                            class="ghost-button"
                            type="button"
                            @click="requestBlock"
                    >
                        拉黑
                    </button>
                    <button
                            :disabled="friendships.actionBusy"
                            class="ghost-button danger-text"
                            type="button"
                            @click="requestDelete"
                    >
                        删除好友
                    </button>
                </div>

                <p v-if="friendships.error" class="form-error" role="alert">
                    {{ friendships.error }}
                    <button
                            class="text-button"
                            type="button"
                            @click="friendships.clearError"
                    >
                        关闭
                    </button>
                </p>
            </section>

            <FriendRequestForm
                    v-if="requestFormOpen"
                    :busy="friendships.actionBusy"
                    :error="friendships.error"
                    :target-name="displayName"
                    @cancel="requestFormOpen = false"
                    @submit="submitFriendRequest"
            />

            <footer v-else class="contact-profile-actions">
                <p v-if="relationshipNotice">{{ relationshipNotice }}</p>
                <div class="contact-profile-primary-actions">
                    <button
                            v-if="canRequest"
                            class="primary-button compact contact-action-button"
                            type="button"
                            @click="openFriendRequestForm"
                    >
                        申请添加好友
                    </button>
                    <template v-else-if="relationship?.has_pending_request">
                        <button
                                class="ghost-button contact-action-button"
                                disabled
                                type="button"
                        >
                            {{
                                relationship.requested_by_me
                                    ? '申请已发送'
                                    : '等待你处理'
                            }}
                        </button>
                    </template>
                    <template v-else>
                        <button
                                :disabled="!canAddToGroup"
                                class="ghost-button contact-action-button"
                                type="button"
                                @click="addToGroup"
                        >
                            添加到群聊
                        </button>
                        <button
                                :disabled="!canStartChat"
                                class="primary-button compact contact-action-button"
                                type="button"
                                @click="startChat"
                        >
                            发私聊
                        </button>
                    </template>
                </div>
            </footer>
        </article>

        <AvatarPreviewDialog
                :avatar-url="avatarUrl"
                :name="displayName"
                :open="avatarPreviewOpen"
                @close="avatarPreviewOpen = false"
        />
    </section>
</template>

<style scoped>
.contact-detail-panel {
    min-width: 0;
    min-height: 0;
    display: flex;
    flex-direction: column;
    background-color: #f1f5f4;
    background-image:
        linear-gradient(rgba(21, 52, 59, 0.035) 1px, transparent 1px),
        linear-gradient(90deg, rgba(21, 52, 59, 0.035) 1px, transparent 1px);
    background-size: 2rem 2rem;
}

.contact-placeholder {
    flex: 1;
    display: grid;
    place-content: center;
    justify-items: center;
    gap: 0.5rem;
    padding: 2rem;
    text-align: center;
}

.contact-placeholder-mark {
    width: 5rem;
    height: 5rem;
    display: grid;
    place-items: center;
    border: 1px solid var(--line);
    border-radius: var(--radius-md);
    color: var(--signal-strong);
    background: color-mix(in srgb, var(--signal-soft) 75%, white);
    font-family: var(--font-display);
    font-size: 2rem;
}

.contact-placeholder h1,
.contact-placeholder p {
    margin: 0;
}

.contact-placeholder h1 {
    font-size: 1.125rem;
}

.contact-placeholder p {
    color: var(--muted);
    font-size: 0.75rem;
}

.contact-profile {
    width: min(46rem, 100%);
    min-height: 100%;
    display: flex;
    flex-direction: column;
    margin: 0 auto;
    padding: clamp(1.5rem, 4vw, 3rem);
}

.contact-profile-header {
    display: flex;
    align-items: center;
    gap: 1.25rem;
}

.contact-profile-avatar {
    width: 7.5rem;
    height: 7.5rem;
    display: grid;
    place-items: center;
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-md);
    padding: 0;
    color: var(--signal-strong);
    background: var(--signal-soft);
    font-family: var(--font-display);
    font-size: 3rem;
    font-weight: 600;
    overflow: hidden;
}

.contact-profile-avatar:hover {
    border-color: var(--signal);
}

.contact-profile-avatar img {
    width: 100%;
    height: 100%;
    object-fit: cover;
}

.contact-profile-heading h1,
.contact-profile-heading p {
    margin: 0;
}

.contact-profile-heading h1 {
    font-size: 1.5rem;
}

.contact-profile-heading p {
    color: var(--muted);
    font-family: var(--font-data);
    font-size: 0.75rem;
}

.contact-relationship-state {
    display: inline-block;
    margin-top: 0.5rem;
    border-radius: 999px;
    padding: 0.1875rem 0.5rem;
    color: var(--signal-strong);
    background: var(--signal-soft);
    font-size: 0.6875rem;
    font-weight: 700;
}

.contact-relationship-state.inactive {
    color: var(--danger);
    background: #f8e8e8;
}

.contact-profile-state {
    margin: 1.5rem 0 0;
    color: var(--muted);
    font-size: 0.8125rem;
}

.contact-profile-fields {
    display: grid;
    gap: 0.75rem;
    margin: 2rem 0 0;
}

.contact-profile-fields > div {
    display: grid;
    grid-template-columns: 6rem minmax(0, 1fr);
    gap: 1rem;
    border-bottom: 1px solid var(--line);
    padding: 0 0 0.875rem;
}

.contact-profile-fields dt {
    color: var(--muted);
    font-size: 0.75rem;
    font-weight: 700;
}

.contact-profile-fields dd {
    margin: 0;
    font-size: 0.8125rem;
    line-height: 1.7;
    white-space: pre-wrap;
}

.contact-remark-value {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 0.75rem;
}

.contact-remark-editor {
    display: grid;
    gap: 0.5rem;
}

.contact-remark-editor input {
    width: 100%;
    border: 1px solid var(--line);
    border-radius: var(--radius-sm);
    padding: 0.625rem 0.75rem;
    color: var(--ink);
    background: white;
}

.contact-remark-editor input:focus {
    border-color: var(--focus);
}

.contact-remark-editor-actions {
    display: flex;
    justify-content: flex-end;
    gap: 0.5rem;
}

.contact-relationship-management {
    display: grid;
    gap: 0.75rem;
    margin-top: 1.5rem;
    border: 1px solid var(--line);
    border-radius: var(--radius-md);
    padding: 1rem;
    background: color-mix(in srgb, var(--surface) 82%, transparent);
}

.contact-relationship-management header h2,
.contact-relationship-management header p,
.contact-confirmation p {
    margin: 0;
}

.contact-relationship-management header h2 {
    font-size: 0.875rem;
}

.contact-relationship-management header p {
    margin-top: 0.25rem;
    color: var(--muted);
    font-size: 0.6875rem;
}

.contact-relationship-buttons,
.contact-confirmation > div {
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: 0.5rem;
}

.contact-confirmation {
    display: grid;
    gap: 0.75rem;
}

.contact-confirmation p {
    color: var(--muted);
    font-size: 0.75rem;
    line-height: 1.7;
}

.contact-confirmation strong {
    color: var(--danger);
}

.contact-profile-actions {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: flex-end;
    gap: 0.625rem;
    margin-top: auto;
    border-top: 1px solid var(--line);
    padding-top: 1rem;
}

.contact-profile-actions p {
    flex-basis: 100%;
    margin: 0;
    color: var(--danger);
    font-size: 0.75rem;
}

.contact-profile-primary-actions {
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: 0.625rem;
    margin-left: auto;
}

.contact-action-button {
    min-width: 7rem;
}

@media (max-width: 48rem) {
    .contact-profile {
        padding: 1.25rem 1rem 5rem;
    }

    .contact-profile-avatar {
        width: 5.5rem;
        height: 5.5rem;
        font-size: 2rem;
    }

    .contact-profile-actions {
        flex-wrap: wrap;
    }

    .contact-profile-primary-actions {
        width: 100%;
    }

    .contact-profile-primary-actions .contact-action-button {
        flex: 1;
    }
}
</style>
