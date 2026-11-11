import http from 'k6/http'
import ws from 'k6/ws'
import {check, sleep} from 'k6'
import {Counter, Rate, Trend} from 'k6/metrics'

const baseURL = __ENV.BASE_URL || 'http://127.0.0.1:8081'
const wsURL = baseURL.replace(/^http/, 'ws')
const fixtureFile =
    __ENV.FIXTURE_FILE || '../../.tmp/group-capacity-fixture.json'
const fixture = JSON.parse(open(fixtureFile))
const memberCount = fixture.members.length
const sendStartSeconds = Number(__ENV.SEND_START_SECONDS || 15)
const deliveryWaitSeconds = Number(__ENV.DELIVERY_WAIT_SECONDS || 60)
const ticketRetryAttempts = Number(__ENV.TICKET_RETRY_ATTEMPTS || 30)

const sendAccepted = new Rate('group_send_accepted')
const sendRateLimited = new Rate('group_send_rate_limited')
const wsConnectionSuccess = new Rate('ws_connection_success')
const wsTicketRateLimited = new Counter('ws_ticket_rate_limited')
const wsDeliveryComplete = new Rate('ws_delivery_complete')
const wsDeliveryCoverage = new Trend('ws_delivery_coverage')
const wsMessagesReceived = new Counter('ws_messages_received')
const wsOwnMessageLatency = new Trend('ws_own_message_latency')

export const options = {
    setupTimeout: '30s',
    scenarios: {
        receivers: {
            executor: 'per-vu-iterations',
            exec: 'receiveMessages',
            vus: memberCount,
            iterations: 1,
            maxDuration: `${sendStartSeconds + deliveryWaitSeconds + 15}s`,
        },
        senders: {
            executor: 'per-vu-iterations',
            exec: 'sendMessage',
            vus: memberCount,
            iterations: 1,
            startTime: `${sendStartSeconds}s`,
            maxDuration: '60s',
        },
    },
    thresholds: {
        group_send_accepted: ['rate>0.99'],
        group_send_rate_limited: ['rate<0.01'],
        'http_req_failed{endpoint:group_send}': ['rate<0.01'],
        // 300 人瞬时突发允许会话行锁排队，但接口仍应在 5 秒内完成
        'http_req_duration{endpoint:group_send}': [
            'p(95)<3500',
            'p(99)<5000',
        ],
        ws_connection_success: ['rate>0.99'],
        ws_delivery_complete: ['rate>0.99'],
        ws_delivery_coverage: ['p(99)>0.99'],
        // 三百人同秒发言属于压力上限，在线扇出尾延迟按 8 秒验收
        ws_own_message_latency: ['p(95)<8000', 'p(99)<12000'],
    },
    summaryTrendStats: ['avg', 'min', 'med', 'p(90)', 'p(95)', 'p(99)', 'max'],
}

// requestHeaders 生成携带 Access Token 的 JSON 请求头
function requestHeaders(token) {
    return {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${token}`,
    }
}

// requestWSTicket 申请一次性 WebSocket Ticket，并读取 Retry-After 恢复
function requestWSTicket(member) {
    for (let attempt = 0; attempt < ticketRetryAttempts; attempt += 1) {
        const response = http.post(
            `${baseURL}/api/v1/auth/ws-ticket`,
            JSON.stringify({
                device_id: member.device_id,
                device_name: 'capacity-test',
                platform: 1,
            }),
            {
                headers: requestHeaders(member.token),
                tags: {endpoint: 'group_ws_ticket'},
            },
        )
        if (response.status === 200) {
            return response.json().ticket
        }
        if (response.status !== 429) {
            throw new Error(
                `申请 WebSocket Ticket失败: status=${response.status} ` +
                    `body=${response.body}`,
            )
        }
        wsTicketRateLimited.add(1)
        const retryAfter = Number(
            response.headers['Retry-After'] ||
                response.headers['Retry-after'] ||
                1,
        )
        sleep(retryAfter + Math.random() * 0.2)
    }
    throw new Error('申请 WebSocket Ticket持续被限流')
}

// setup 计算统一发送时刻和在线连接保持截止时间
export function setup() {
    const now = Date.now()
    return {
        fixture,
        sendAt: now + sendStartSeconds * 1000,
        holdUntil:
            now + (sendStartSeconds + deliveryWaitSeconds) * 1000,
    }
}

// receiveMessages 保持 WebSocket 在线并统计本群消息覆盖与本人消息延迟
export function receiveMessages(data) {
    const memberIndex = (__VU - 1) % data.fixture.members.length
    const member = data.fixture.members[memberIndex]
    let ticket
    try {
        ticket = requestWSTicket(member)
    } catch (error) {
        console.error(error.message)
        wsConnectionSuccess.add(false)
        wsDeliveryComplete.add(false)
        wsDeliveryCoverage.add(0)
        return
    }

    const receivedMessageIDs = new Set()
    let connected = false
    let ownLatencyRecorded = false
    const timeout = Math.max(1000, data.holdUntil - Date.now())

    const response = ws.connect(
        `${wsURL}/ws?ticket=${encodeURIComponent(ticket)}`,
        {tags: {endpoint: 'group_capacity_ws'}},
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
                if (
                    message.action !== 'message' ||
                    message.conversation_id !== data.fixture.conversation_id
                ) {
                    return
                }
                receivedMessageIDs.add(message.message_id)
                wsMessagesReceived.add(1)
                if (
                    !ownLatencyRecorded &&
                    message.sender_id === member.user_id
                ) {
                    const match = /sentAt=(\d+)/.exec(message.content || '')
                    if (match) {
                        ownLatencyRecorded = true
                        wsOwnMessageLatency.add(
                            Math.max(0, Date.now() - Number(match[1])),
                        )
                    }
                }
            })
            socket.on('error', () => {
                if (!connected) {
                    wsConnectionSuccess.add(false)
                }
            })
            socket.setTimeout(() => socket.close(), timeout)
        },
    )

    check(response, {
        '容量群 WebSocket 握手成功': (result) =>
            result && result.status === 101,
    })
    if (!connected) {
        wsConnectionSuccess.add(false)
    }
    const receivedCount = receivedMessageIDs.size
    wsDeliveryCoverage.add(receivedCount / memberCount)
    wsDeliveryComplete.add(receivedCount >= memberCount)
}

// sendMessage 在统一时刻由每个群成员发送一条消息
export function sendMessage(data) {
    const memberIndex = (__VU - 1) % data.fixture.members.length
    const member = data.fixture.members[memberIndex]
    const waitSeconds = Math.max(0, data.sendAt - Date.now()) / 1000
    if (waitSeconds > 0) {
        sleep(waitSeconds)
    }

    const sentAt = Date.now()
    const response = http.post(
        `${baseURL}/api/v1/conversations/` +
            `${data.fixture.conversation_id}/messages`,
        JSON.stringify({
            client_message_id:
                `capacity-${data.fixture.run_id}-${memberIndex}-${sentAt}`,
            content:
                `RelayIM 同群容量测试 member=${memberIndex} sentAt=${sentAt}`,
        }),
        {
            headers: requestHeaders(member.token),
            tags: {endpoint: 'group_send'},
        },
    )
    sendAccepted.add(response.status === 201)
    sendRateLimited.add(response.status === 429)
    check(response, {
        '容量群消息 Accepted': (result) => result.status === 201,
        '容量群消息返回 seq': (result) => {
            if (result.status !== 201) return false
            return Number(result.json().seq) > 0
        },
    })
}

// handleSummary 输出容量场景指标，不包含 Token 和用户 ID
export function handleSummary(data) {
    const requiredMetrics = [
        'checks',
        'http_req_failed',
        'http_req_failed{endpoint:group_send}',
        'http_reqs',
        'http_reqs{endpoint:group_send}',
        'http_req_duration',
        'http_req_duration{endpoint:group_send}',
        'group_send_accepted',
        'group_send_rate_limited',
        'ws_connection_success',
        'ws_ticket_rate_limited',
        'ws_delivery_complete',
        'ws_delivery_coverage',
        'ws_messages_received',
        'ws_own_message_latency',
        'ws_sessions',
    ]
    const metrics = {}
    for (const name of requiredMetrics) {
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
            groupMembers: memberCount,
            simultaneousSends: memberCount,
            onlineWebSockets: memberCount,
            sendStartSeconds,
            deliveryWaitSeconds,
        },
        metrics,
    }
    const outputPath =
        `loadtest/reports/${__ENV.SUMMARY_FILE || 'group-capacity.json'}`
    return {
        [outputPath]: JSON.stringify(payload, null, 2),
    }
}
