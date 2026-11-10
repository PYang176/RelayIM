<script setup lang="ts">
import {nextTick, shallowRef, useTemplateRef, watch} from 'vue'

import AddFriendPanel from '@/components/AddFriendPanel.vue'
import FriendRequestRow from '@/components/FriendRequestRow.vue'
import FriendRow from '@/components/FriendRow.vue'
import {useFriendshipStore} from '@/stores/friendships'

const props = defineProps<{
    open: boolean
}>()

const emit = defineEmits<{
    close: []
    openDirect: [userID: string]
    viewProfile: [userID: string]
    friendRemoved: [userID: string]
}>()

const dialog = useTemplateRef<HTMLDialogElement>('dialog')
const closeButton = useTemplateRef<HTMLButtonElement>('closeButton')
const friendships = useFriendshipStore()
const activeTab = shallowRef<'friends' | 'requests' | 'blocked'>('friends')
const activeView = shallowRef<'contacts' | 'add'>('contacts')
const requestMode = shallowRef<'received' | 'sent'>('received')

watch(
    () => props.open,
    async (open) => {
        if (open) {
            activeView.value = 'contacts'
            dialog.value?.showModal()
            await friendships.loadAll()
            await nextTick()
            closeButton.value?.focus()
        } else {
            dialog.value?.close()
        }
    },
    {immediate: true},
)

// close 关闭联系人抽屉
function close() {
    activeView.value = 'contacts'
    friendships.clearSearch()
    emit('close')
}

// openAddFriend 切换到添加好友视图
function openAddFriend() {
    friendships.clearError()
    friendships.clearSearch()
    activeView.value = 'add'
}

// backToContacts 返回好友列表视图并清理搜索结果
function backToContacts() {
    friendships.clearError()
    friendships.clearSearch()
    activeView.value = 'contacts'
}

// removeFriend 单向删除好友并同步当前私聊权限
async function removeFriend(friendUserID: string) {
    if (await friendships.removeFriend(friendUserID)) {
        emit('friendRemoved', friendUserID)
    }
}
</script>

<template>
    <dialog
            ref="dialog"
            class="friends-dialog"
            aria-labelledby="friends-title"
            @cancel.prevent="close"
            @close="close"
    >
        <div class="friends-drawer">
            <header class="friends-header">
                <div>
                    <h2 id="friends-title">
                        {{ activeView === 'add' ? '添加好友' : '联系人' }}
                    </h2>
                    <p>
                        {{
                            activeView === 'add'
                                ? '查找用户并填写理由发送好友申请。'
                                : '处理好友申请、备注和既有联系人。'
                        }}
                    </p>
                </div>
                <div class="friends-header-actions">
                    <button
                            v-if="activeView === 'contacts'"
                            class="primary-button compact"
                            type="button"
                            @click="openAddFriend"
                    >
                        添加好友
                    </button>
                    <button
                            v-else
                            class="ghost-button"
                            type="button"
                            @click="backToContacts"
                    >
                        返回联系人
                    </button>
                    <button
                            ref="closeButton"
                            class="icon-button"
                            aria-label="关闭联系人"
                            type="button"
                            @click="close"
                    >
                        ×
                    </button>
                </div>
            </header>

            <div
                    v-if="activeView === 'contacts'"
                    class="friends-tabs"
                    role="group"
                    aria-label="联系人页面"
            >
                <button
                        :aria-pressed="activeTab === 'friends'"
                        :class="{ active: activeTab === 'friends' }"
                        type="button"
                        @click="activeTab = 'friends'"
                >
                    好友 {{ friendships.friends.length
                    }}{{ friendships.friendHasMore ? '+' : '' }}
                </button>
                <button
                        :aria-pressed="activeTab === 'requests'"
                        :class="{ active: activeTab === 'requests' }"
                        type="button"
                        @click="activeTab = 'requests'"
                >
                    申请 {{ friendships.receivedRequests.length
                    }}{{ friendships.receivedHasMore ? '+' : '' }}
                </button>
                <button
                        :aria-pressed="activeTab === 'blocked'"
                        :class="{ active: activeTab === 'blocked' }"
                        type="button"
                        @click="activeTab = 'blocked'"
                >
                    拉黑 {{ friendships.blockedUsers.length
                    }}{{ friendships.blockedHasMore ? '+' : '' }}
                </button>
            </div>

            <p
                    v-if="activeView === 'contacts' && friendships.error"
                    class="form-error"
                    role="alert"
            >
                {{ friendships.error }}
                <button class="text-button" type="button" @click="friendships.clearError">
                    关闭
                </button>
            </p>

            <AddFriendPanel
                    v-if="activeView === 'add'"
                    @view-profile="emit('viewProfile', $event)"
            />

            <template v-else-if="activeTab === 'friends'">
                <section class="drawer-section drawer-list" aria-live="polite">
                    <h3>好友</h3>
                    <p v-if="friendships.loading" role="status">加载好友中…</p>
                    <p v-else-if="friendships.friends.length === 0" role="status">
                        还没有好友。搜索用户并发送申请。
                    </p>
                    <FriendRow
                            v-for="friend in friendships.friends"
                            :key="friend.friendship_id"
                            :busy="friendships.actionBusy"
                            :friend="friend"
                            @message="emit('openDirect', $event)"
                            @profile="emit('viewProfile', $event)"
                            @block="friendships.blockUser(friend.user.id)"
                            @remove="removeFriend(friend.user.id)"
                            @unblock="friendships.unblockUser(friend.user.id)"
                            @update-remark="friendships.updateRemark(friend.user.id, $event)"
                    />
                    <button
                            v-if="friendships.friendHasMore"
                            class="text-button"
                            :disabled="friendships.loadingMore"
                            type="button"
                            @click="friendships.loadMoreFriends"
                    >
                        {{ friendships.loadingMore ? '加载中…' : '加载更多好友' }}
                    </button>
                </section>
            </template>

            <template v-else-if="activeTab === 'requests'">
                <div class="friends-tabs compact" role="group" aria-label="申请方向">
                    <button
                            :aria-pressed="requestMode === 'received'"
                            :class="{ active: requestMode === 'received' }"
                            type="button"
                            @click="requestMode = 'received'"
                    >
                        收到
                    </button>
                    <button
                            :aria-pressed="requestMode === 'sent'"
                            :class="{ active: requestMode === 'sent' }"
                            type="button"
                            @click="requestMode = 'sent'"
                    >
                        发出
                    </button>
                </div>

                <section class="drawer-section drawer-list" aria-live="polite">
                    <p
                            v-if="
              requestMode === 'received'
                ? friendships.receivedRequests.length === 0
                : friendships.sentRequests.length === 0
            "
                            role="status"
                    >
                        暂无{{ requestMode === 'received' ? '收到' : '发出' }}的好友申请。
                    </p>
                    <FriendRequestRow
                            v-for="request in requestMode === 'received'
              ? friendships.receivedRequests
              : friendships.sentRequests"
                            :key="request.id"
                            :busy="friendships.actionBusy"
                            :received="requestMode === 'received'"
                            :request="request"
                            @accept="friendships.acceptRequest(request.id)"
                            @reject="friendships.rejectRequest(request.id)"
                    />
                    <button
                            v-if="
                                requestMode === 'received'
                                    ? friendships.receivedHasMore
                                    : friendships.sentHasMore
                            "
                            class="text-button"
                            :disabled="friendships.loadingMore"
                            type="button"
                            @click="
                                requestMode === 'received'
                                    ? friendships.loadMoreReceivedRequests()
                                    : friendships.loadMoreSentRequests()
                            "
                    >
                        {{ friendships.loadingMore ? '加载中…' : '加载更多申请' }}
                    </button>
                </section>
            </template>

            <section v-else class="drawer-section drawer-list" aria-live="polite">
                <h3>已拉黑用户</h3>
                <p v-if="friendships.blockedUsers.length === 0" role="status">
                    当前没有拉黑用户。
                </p>
                <article
                        v-for="item in friendships.blockedUsers"
                        :key="item.user.id"
                        class="request-row"
                >
                    <span class="friend-avatar">
                        {{ (item.user.nickname || item.user.username).slice(0, 1) }}
                    </span>
                    <div class="friend-copy">
                        <strong>{{ item.user.nickname || item.user.username }}</strong>
                        <small>@{{ item.user.username }}</small>
                    </div>
                    <button
                            :disabled="friendships.actionBusy"
                            type="button"
                            @click="friendships.unblockUser(item.user.id)"
                    >
                        取消拉黑
                    </button>
                </article>
                <button
                        v-if="friendships.blockedHasMore"
                        class="text-button"
                        :disabled="friendships.loadingMore"
                        type="button"
                        @click="friendships.loadMoreBlockedUsers"
                >
                    {{ friendships.loadingMore ? '加载中…' : '加载更多拉黑用户' }}
                </button>
            </section>
        </div>
    </dialog>
</template>

<style scoped>
.friends-header-actions {
    display: flex;
    align-items: center;
    gap: 0.5rem;
}
</style>
