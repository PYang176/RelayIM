<script setup lang="ts">
import { nextTick, shallowRef, useTemplateRef, watch } from 'vue'

import FriendRequestRow from '@/components/FriendRequestRow.vue'
import FriendRow from '@/components/FriendRow.vue'
import { useFriendshipStore } from '@/stores/friendships'

const props = defineProps<{
  open: boolean
}>()

const emit = defineEmits<{
  close: []
  openDirect: [userID: string]
}>()

const dialog = useTemplateRef<HTMLDialogElement>('dialog')
const closeButton = useTemplateRef<HTMLButtonElement>('closeButton')
const friendships = useFriendshipStore()
const activeTab = shallowRef<'friends' | 'requests'>('friends')
const searchQuery = shallowRef('')
const requestMode = shallowRef<'received' | 'sent'>('received')

watch(
  () => props.open,
  async (open) => {
    if (open) {
      dialog.value?.showModal()
      await friendships.loadAll()
      await nextTick()
      closeButton.value?.focus()
    } else {
      dialog.value?.close()
    }
  },
  { immediate: true },
)

async function submitSearch() {
  await friendships.search(searchQuery.value)
}

function close() {
  emit('close')
}

function sendRequest(userID: string) {
  void friendships.sendRequest(userID, '')
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
          <h2 id="friends-title">联系人</h2>
          <p>搜索用户、处理申请和设置独立好友备注。</p>
        </div>
        <button
          ref="closeButton"
          class="icon-button"
          aria-label="关闭联系人"
          type="button"
          @click="close"
        >
          ×
        </button>
      </header>

      <div class="friends-tabs" role="group" aria-label="联系人页面">
        <button
          :aria-pressed="activeTab === 'friends'"
          :class="{ active: activeTab === 'friends' }"
          type="button"
          @click="activeTab = 'friends'"
        >
          好友 {{ friendships.friends.length }}
        </button>
        <button
          :aria-pressed="activeTab === 'requests'"
          :class="{ active: activeTab === 'requests' }"
          type="button"
          @click="activeTab = 'requests'"
        >
          申请 {{ friendships.receivedRequests.length }}
        </button>
      </div>

      <p v-if="friendships.error" class="form-error" role="alert">
        {{ friendships.error }}
        <button class="text-button" type="button" @click="friendships.clearError">
          关闭
        </button>
      </p>

      <template v-if="activeTab === 'friends'">
        <form class="friend-search" role="search" @submit.prevent="submitSearch">
          <label class="sr-only" for="friend-search-input">搜索用户</label>
          <input
            id="friend-search-input"
            v-model="searchQuery"
            maxlength="50"
            placeholder="输入用户名或昵称"
            required
          />
          <button :disabled="friendships.searching" type="submit">
            {{ friendships.searching ? '搜索中' : '搜索' }}
          </button>
        </form>

        <section v-if="friendships.searchResults.length" class="drawer-section">
          <h3>搜索结果</h3>
          <article
            v-for="user in friendships.searchResults"
            :key="user.id"
            class="request-row"
          >
            <span class="friend-avatar">
              {{ (user.nickname || user.username).slice(0, 1) }}
            </span>
            <div class="friend-copy">
              <strong>{{ user.nickname || user.username }}</strong>
              <small>@{{ user.username }}</small>
            </div>
            <button
              :disabled="friendships.actionBusy"
              type="button"
              @click="sendRequest(user.id)"
            >
              添加
            </button>
          </article>
        </section>

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
            @remove="friendships.removeFriend(friend.user.id)"
            @update-remark="friendships.updateRemark(friend.user.id, $event)"
          />
        </section>
      </template>

      <template v-else>
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
        </section>
      </template>
    </div>
  </dialog>
</template>
