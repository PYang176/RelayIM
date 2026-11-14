<template>
    <div class="workspace-preview" aria-hidden="true">
        <aside class="preview-rail">
            <span class="preview-brand">R</span>
            <span class="preview-rail-item active">聊</span>
            <span class="preview-rail-item">联</span>
            <span class="preview-rail-item">邀</span>
            <span class="preview-user">岚</span>
        </aside>
        <section class="preview-inbox">
            <div class="preview-inbox-head">
                <strong>消息</strong>
                <span>+</span>
            </div>
            <div class="preview-search">搜索会话</div>
            <article class="preview-conversation active">
                <span class="preview-avatar">产</span>
                <div>
                    <strong>产品讨论组</strong>
                    <small>新版本体验已经准备好了</small>
                </div>
                <time>10:24</time>
            </article>
            <article class="preview-conversation">
                <span class="preview-avatar mint">北</span>
                <div>
                    <strong>北辰</strong>
                    <small>我把文件放到群里了</small>
                </div>
                <time>昨天</time>
            </article>
            <article class="preview-conversation">
                <span class="preview-avatar warm">清</span>
                <div>
                    <strong>清禾</strong>
                    <small>周六一起过一遍流程</small>
                </div>
                <time>周二</time>
            </article>
        </section>
        <section class="preview-chat">
            <header class="preview-chat-head">
                <div>
                    <strong>产品讨论组</strong>
                    <small>6 人在线</small>
                </div>
                <span>聊天记录</span>
            </header>
            <div class="preview-day">今天</div>
            <article class="preview-message incoming">
                <span class="preview-avatar mint">北</span>
                <div>
                    <small>北辰</small>
                    <p>首页的新方案已经放到共享文件里了。</p>
                </div>
            </article>
            <article class="preview-message incoming">
                <span class="preview-avatar warm">清</span>
                <div>
                    <small>清禾</small>
                    <p>收到，我看了第一版，信息层级清楚很多。</p>
                </div>
            </article>
            <article class="preview-message outgoing">
                <div>
                    <p>那就按这个方向继续。</p>
                    <small>已读</small>
                </div>
            </article>
            <article class="preview-file">
                <span>PDF</span>
                <div>
                    <strong>RelayIM 体验方案.pdf</strong>
                    <small>2.4 MB</small>
                </div>
            </article>
            <div class="preview-composer">输入消息，Enter 发送</div>
        </section>
    </div>
</template>
