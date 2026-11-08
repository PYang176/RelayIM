const baseURL1 = process.env.GINCHAT_E2E_BASE_URL_1 ?? 'http://127.0.0.1:18081'
const baseURL2 = process.env.GINCHAT_E2E_BASE_URL_2 ?? 'http://127.0.0.1:18082'
const userA = process.env.GINCHAT_E2E_USER_A ?? 'demo_alice_e2e'
const userB = process.env.GINCHAT_E2E_USER_B ?? 'demo_bob_e2e'
const password = process.env.GINCHAT_E2E_PASSWORD ?? 'Demo123456!'

async function call(baseURL, path, options = {}, token = '') {
    const headers = new Headers(options.headers)
    if (options.body) {
        headers.set('Content-Type', 'application/json')
    }
    if (token) {
        headers.set('Authorization', `Bearer ${token}`)
    }
    const response = await fetch(`${baseURL}/api/v1${path}`, {...options, headers})
    if (!response.ok) {
        throw new Error(`${response.status} ${await response.text()}`)
    }
    return response.status === 204 ? null : response.json()
}

async function login(baseURL, username, deviceName) {
    return call(baseURL, '/auth/login', {
        method: 'POST',
        body: JSON.stringify({
            username,
            password,
            device_name: deviceName,
            platform: 4,
        }),
    })
}

async function connect(baseURL, session) {
    const ticket = await call(
        baseURL,
        '/auth/ws-ticket',
        {
            method: 'POST',
            body: JSON.stringify({
                device_id: session.device_id,
                device_name: 'two-instance-e2e',
                platform: 4,
            }),
        },
        session.token,
    )
    const socket = new WebSocket(
        `${baseURL.replace('http://', 'ws://')}/ws?ticket=${encodeURIComponent(ticket.ticket)}`,
    )
    const received = []
    socket.addEventListener('message', (event) => {
        try {
            received.push(JSON.parse(event.data))
        } catch {
            // 测试只关心合法 JSON 帧。
        }
    })
    await new Promise((resolve, reject) => {
        socket.addEventListener('open', resolve, {once: true})
        socket.addEventListener('error', reject, {once: true})
    })
    return {socket, received}
}

async function waitForAction(client, messageID, action, timeout = 5000) {
    const deadline = Date.now() + timeout
    while (Date.now() < deadline) {
        const event = client.received.find(
            (item) => item.message_id === messageID && item.action === action,
        )
        if (event) return event
        await new Promise((resolve) => setTimeout(resolve, 25))
    }
    throw new Error(`等待 ${action} ${messageID} 超时`)
}

const alice1 = await login(baseURL1, userA, 'Alice 实例 1')
const alice2 = await login(baseURL2, userA, 'Alice 实例 2')
const bob2 = await login(baseURL2, userB, 'Bob 实例 2')

const conversations = await call(
    baseURL1,
    '/conversations',
    {},
    alice1.token,
)
const direct = conversations.conversations.find(
    (item) =>
        item.conversation_type === 1 &&
        item.peer_user_id === bob2.user.id,
)
if (!direct) {
    throw new Error('未找到 Alice 与 Bob 的私聊，请先运行 scripts/seed-demo.ps1')
}

const clients = await Promise.all([
    connect(baseURL1, alice1),
    connect(baseURL2, alice2),
    connect(baseURL2, bob2),
])
await new Promise((resolve) => setTimeout(resolve, 300))

const startedAt = Date.now()
const sent = await call(
    baseURL1,
    `/conversations/${direct.id}/messages`,
    {
        method: 'POST',
        body: JSON.stringify({
            client_message_id: crypto.randomUUID(),
            content: `跨实例多设备验证 ${startedAt}`,
        }),
    },
    alice1.token,
)

const delivered = await Promise.all(
    clients.map((client) =>
        waitForAction(client, sent.message_id, 'message'),
    ),
)

await call(
    baseURL1,
    `/conversations/${direct.id}/messages/${sent.message_id}`,
    {method: 'DELETE'},
    alice1.token,
)
const recalled = await Promise.all(
    clients.map((client) =>
        waitForAction(client, sent.message_id, 'message_recalled'),
    ),
)

const result = {
    message_id: sent.message_id,
    delivery_latency_ms: Date.now() - startedAt,
    delivered_to: delivered.length,
    recalled_on: recalled.length,
    sender_device_on_instance_1: delivered[0].action === 'message',
    recipient_on_instance_2: delivered[2].action === 'message',
    same_user_second_device: delivered[1].action === 'message',
}

for (const client of clients) {
    client.socket.close(1000, 'two-instance e2e complete')
}
console.log(JSON.stringify(result))
if (
    !result.sender_device_on_instance_1 ||
    !result.recipient_on_instance_2 ||
    !result.same_user_second_device
) {
    process.exit(1)
}
