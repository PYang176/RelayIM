import http from 'k6/http'
import ws from 'k6/ws'
import {check, fail, sleep} from 'k6'
import {Counter, Rate, Trend} from 'k6/metrics'

const baseURL = __ENV.BASE_URL || 'http://127.0.0.1:8081'
const wsURL = baseURL.replace(/^http/, 'ws')
const password = 'Load-test-123456'
const runID = `${Date.now().toString(36)}${Math.floor(Math.random() * 100000)}`
const receiverCount = Number(__ENV.WS_RECEIVERS || 20)
const durationSeconds = Number(__ENV.DURATION_SECONDS || 30)
const senderStartSeconds = Number(__ENV.SENDER_START_SECONDS || 3)
const senderDurationSeconds = Math.max(
    1,
    durationSeconds - senderStartSeconds - 3,
)

const wsConnectionSuccess = new Rate('ws_connection_success')
const wsMessagesReceived = new Counter('ws_messages_received')
const wsDeliveryLatency = new Trend('ws_delivery_latency')

export const options = {
    setupTimeout: '2m',
    scenarios: {
        receivers: {
            executor: 'constant-vus',
            exec: 'receiveMessages',
            vus: receiverCount,
            duration: `${durationSeconds}s`,
        },
        senders: {
            executor: 'constant-vus',
            exec: 'sendMessages',
            vus: 1,
            startTime: `${senderStartSeconds}s`,
            duration: `${senderDurationSeconds}s`,
        },
    },
    thresholds: {
        checks: ['rate>0.99'],
        http_req_failed: ['rate<0.01'],
        ws_connection_success: ['rate>0.99'],
        ws_messages_received: ['count>0'],
        ws_delivery_latency: ['p(95)<500', 'p(99)<1000'],
    },
    summaryTrendStats: ['avg', 'min', 'med', 'p(90)', 'p(95)', 'p(99)', 'max'],
}

function requestHeaders(token) {
    return {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${token}`,
    }
}

function requireStatus(response, expected, action) {
    if (response.status !== expected) {
        fail(`${action}失败: status=${response.status} body=${response.body}`)
    }
}

function register(username, nickname) {
    const response = http.post(
        `${baseURL}/api/v1/auth/register`,
        JSON.stringify({
            username,
            password,
            confirm_password: password,
            nickname,
        }),
        {headers: {'Content-Type': 'application/json'}},
    )
    requireStatus(response, 201, `注册 ${username}`)
    return response.json()
}

function login(username) {
    const response = http.post(
        `${baseURL}/api/v1/auth/login`,
        JSON.stringify({username, password}),
        {headers: {'Content-Type': 'application/json'}},
    )
    requireStatus(response, 200, `登录 ${username}`)
    return response.json()
}

function acceptFriendRequest(receiverToken, senderID) {
    const received = http.get(
        `${baseURL}/api/v1/friend-requests/received`,
        {headers: requestHeaders(receiverToken)},
    )
    requireStatus(received, 200, '查询 WebSocket 压测好友申请')
    const request = received
        .json()
        .requests.find((item) => item.user.id === senderID)
    if (!request) {
        fail('未找到 WebSocket 压测好友申请')
    }
    requireStatus(
        http.post(
            `${baseURL}/api/v1/friend-requests/${request.id}/accept`,
            null,
            {headers: requestHeaders(receiverToken)},
        ),
        200,
        '同意 WebSocket 压测好友申请',
    )
}

export function setup() {
    const senderUsername = `loadwss_${runID}`
    const sender = register(senderUsername, 'WebSocket 压测发送者')
    const senderSession = login(senderUsername)
    const receivers = []
    const memberIDs = []

    for (let index = 0; index < receiverCount; index += 1) {
        const username = `loadwsr_${runID}_${index}`
        const receiver = register(username, `WS接收者${index + 1}`)
        const session = login(username)
        requireStatus(
            http.post(
                `${baseURL}/api/v1/friend-requests`,
                JSON.stringify({
                    target_user_id: receiver.id,
                    remark: 'k6 WebSocket baseline',
                }),
                {headers: requestHeaders(senderSession.token)},
            ),
            201,
            '发送 WebSocket 压测好友申请',
        )
        acceptFriendRequest(
            session.token,
            sender.id,
        )
        receivers.push({
            username,
            token: session.token,
        })
        memberIDs.push(receiver.id)
    }

    const groupResponse = http.post(
        `${baseURL}/api/v1/conversations/groups`,
        JSON.stringify({
            name: `WebSocket 基线群 ${runID}`,
            member_user_ids: memberIDs,
        }),
        {headers: requestHeaders(senderSession.token)},
    )
    requireStatus(groupResponse, 201, '创建 WebSocket 压测群聊')

    return {
        runID,
        senderUsername,
        senderToken: senderSession.token,
        conversationID: groupResponse.json().id,
        receivers,
    }
}

export function receiveMessages(data) {
    const receiverIndex =
        (__VU - 1) % data.receivers.length
    const receiver = data.receivers[receiverIndex]
    const ticketResponse = http.post(
        `${baseURL}/api/v1/auth/ws-ticket`,
        JSON.stringify({
            device_name: `k6-ws-${__VU}`,
            platform: 1,
        }),
        {headers: requestHeaders(receiver.token)},
    )
    requireStatus(ticketResponse, 200, '申请 WebSocket Ticket')
    const ticket = ticketResponse.json().ticket
    let connected = false

    const response = ws.connect(
        `${wsURL}/ws?ticket=${encodeURIComponent(ticket)}`,
        {tags: {endpoint: 'ws_delivery'}},
        function (socket) {
            socket.on('open', () => {
                connected = true
                wsConnectionSuccess.add(true)
            })
            socket.on('message', (rawMessage) => {
                let message
                try {
                    message = JSON.parse(rawMessage)
                } catch {
                    return
                }
                if (message.action !== 'message') {
                    return
                }
                wsMessagesReceived.add(1)
                const match = /sentAt=(\d+)/.exec(message.content || '')
                if (match) {
                    wsDeliveryLatency.add(
                        Math.max(0, Date.now() - Number(match[1])),
                    )
                }
                socket.send(JSON.stringify({
                    action: 'ack',
                    message_id: message.message_id,
                }))
                socket.send(JSON.stringify({
                    action: 'read',
                    message_id: '',
                    conversation_id: message.conversation_id,
                    seq: message.seq,
                }))
            })
            socket.on('error', () => {
                if (!connected) {
                    wsConnectionSuccess.add(false)
                }
            })
            socket.setTimeout(() => socket.close(), durationSeconds * 1000)
        },
    )
    check(response, {
        'WebSocket 握手成功': (result) => result && result.status === 101,
    })
    if (!connected) {
        wsConnectionSuccess.add(false)
    }
}

export function sendMessages(data) {
    const deadline = Date.now() + senderDurationSeconds * 1000
    let sequence = 0
    while (Date.now() < deadline) {
        const sentAt = Date.now()
        const clientMessageID =
            `ws-${data.runID}-${__VU}-${sequence}-${sentAt}`
        const response = http.post(
            `${baseURL}/api/v1/conversations/${data.conversationID}/messages`,
            JSON.stringify({
                client_message_id: clientMessageID,
                content:
                    `k6 WebSocket 基线消息 run=${data.runID} ` +
                    `seq=${sequence} sentAt=${sentAt}`,
            }),
            {
                headers: requestHeaders(data.senderToken),
                tags: {endpoint: 'send_ws_message'},
            },
        )
        check(response, {
            'WebSocket 场景消息 Accepted': (result) => result.status === 201,
        })
        sequence += 1
        sleep(Number(__ENV.SEND_INTERVAL_SECONDS || 0.2))
    }
}

export function handleSummary(data) {
    const requiredMetrics = [
        'checks',
        'http_req_failed',
        'http_reqs',
        'ws_connection_success',
        'ws_messages_received',
        'ws_delivery_latency',
        'ws_sessions',
    ]
    const metricNames = Object.keys(data.metrics).filter(
        (name) =>
            requiredMetrics.includes(name) ||
            name.startsWith('http_req_duration'),
    )
    const metrics = {}
    for (const name of metricNames) {
        const metric = data.metrics[name]
        if (!metric) continue
        metrics[name] = {
            type: metric.type,
            values: metric.values,
            thresholds: metric.thresholds || {},
        }
    }
    const payload = {
        scenario: {
            receiverVUs: receiverCount,
            durationSeconds,
            senderStartSeconds,
            senderDurationSeconds,
            sendIntervalSeconds: Number(
                __ENV.SEND_INTERVAL_SECONDS || 0.2,
            ),
        },
        metrics,
    }
    const outputPath =
        `loadtest/reports/${__ENV.SUMMARY_FILE || 'websocket-baseline.json'}`
    return {
        [outputPath]: JSON.stringify(payload, null, 2),
    }
}
