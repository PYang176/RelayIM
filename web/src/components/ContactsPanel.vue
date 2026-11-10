<script setup lang="ts">
import {onMounted, shallowRef} from 'vue'

import AddFriendPanel from '@/components/AddFriendPanel.vue'
import FriendRequestRow from '@/components/FriendRequestRow.vue'
import FriendRow from '@/components/FriendRow.vue'
import {useFriendshipStore} from '@/stores/friendships'

const props = defineProps<{
    selectedFriendId: string
}>()

const emit = defineEmits<{
    selectFriend: [userID: string]
    friendRemoved: [userID: string]
    openDirect: [userID: string]
}>()

const friendships = useFriendshipStore()
const activeTab = shallowRef<'friends' | 'requests' | 'blocked'>('friends')
const activeView = shallowRef<'contacts' | 'add'>('contacts')
const requestMode = shallowRef<'received' | 'sent'>('received')

onMounted(() => {
    void friendships.loadAll()
})

// openAddFriend 切换到添加好友视图
function openAddFriend() {
    activeView.value = 'add'
}

// backToContacts 返回联系人列表并清理搜索状态
function backToContacts() {
    friendships.clearError()
    friendships.clearSearch()
    activeView.value = 'contacts'
}

// removeFriend 单向删除好友并从详情区清空选择
async function removeFriend(friendUserID: string) {
    if (await friendships.removeFriend(friendUserID)) {
        emit('friendRemoved', friendUserID)
    }
}
</script>

<template>
    <aside class="contacts-panel" aria-label="联系人列表">
        <header class="contacts-panel-header">
            <div>
                <h2>{{ activeView === 'add' ? '添加好友' : '联系人' }}</h2>
                <p>
                    {{
                        activeView === 'add'
                            ? '查找用户并发送好友申请'
                            : '选择联系人查看资料'
                    }}
                </p>
            </div>
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
                返回
            </button>
        </header>

        <template v-if="activeView === 'contacts'">
            <div class="friends-tabs" role="group" aria-label="联系人页面">
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

            <p v-if="friendships.error" class="form-error" role="alert">
                {{ friendships.error }}
                <button class="text-button" type="button" @click="friendships.clearError">
                    关闭
                </button>
            </p>

            <section
                    v-if="activeTab === 'friends'"
                    class="drawer-section contacts-scroll"
                    aria-live="polite"
            >
                <p v-if="friendships.loading" role="status">加载好友中…</p>
                <p v-else-if="friendships.friends.length === 0" role="status">
                    还没有好友，点击“添加好友”开始。
                </p>
                <FriendRow
                        v-for="friend in friendships.friends"
                        :key="friend.friendship_id"
                        :busy="friendships.actionBusy"
                        :friend="friend"
                        :selected="friend.user.id === props.selectedFriendId"
                        @block="friendships.blockUser(friend.user.id)"
                        @message="emit('openDirect', friend.user.id)"
                        @remove="removeFriend(friend.user.id)"
                        @select="emit('selectFriend', friend.user.id)"
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

            <section
                    v-else-if="activeTab === 'requests'"
                    class="drawer-section contacts-scroll"
                    aria-live="polite"
            >
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
                <p
                        v-if="
                            requestMode === 'received'
                                ? friendships.receivedRequests.length === 0
                                : friendships.sentRequests.length === 0
                        "
                        role="status"
                >
                    暂无{{ requestMode === 'received' ? '收到' : '发出' }}的好友申请
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

            <section
                    v-else
                    class="drawer-section contacts-scroll"
                    aria-live="polite"
            >
                <h3>已拉黑用户</h3>
                <p v-if="friendships.blockedUsers.length === 0" role="status">
                    当前没有拉黑用户
                </p>
                <article
                        v-for="item in friendships.blockedUsers"
                        :key="item.user.id"
                        class="request-row"
                >
                    <span class="friend-avatar">
                        <img
                                v-if="item.user.avatar_url"
                                :alt="item.user.nickname || item.user.username"
                                :src="item.user.avatar_url"
                        />
                        <template v-else>
                            {{ (item.user.nickname || item.user.username).slice(0, 1) }}
                        </template>
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
        </template>

        <AddFriendPanel v-else />
    </aside>
</template>

<style scoped>
.contacts-panel {
    min-width: 0;
    min-height: 0;
    display: flex;
    flex-direction: column;
    gap: 0.875rem;
    border-right: 1px solid var(--line);
    padding: 0.875rem 1rem;
    background: #f5f8f8;
}

.contacts-panel-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 0.75rem;
}

.contacts-panel-header h2,
.contacts-panel-header p {
    margin: 0;
}

.contacts-panel-header h2 {
    font-size: 1.125rem;
    font-weight: 700;
}

.contacts-panel-header p {
    color: var(--muted);
    font-size: 0.6875rem;
}

.contacts-scroll {
    min-height: 0;
    overflow-y: auto;
    align-content: start;
    padding-right: 0.25rem;
}

@media (max-width: 48rem) {
    .contacts-panel {
        max-height: 14rem;
        border-right: 0;
        border-bottom: 1px solid var(--line);
        padding: 0.75rem;
    }

    .contacts-scroll {
        max-height: 7.5rem;
    }
}
</style>
