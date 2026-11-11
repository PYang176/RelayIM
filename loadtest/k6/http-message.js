import http from 'k6/http'
import {check, fail, sleep} from 'k6'

const baseURL = __ENV.BASE_URL || 'http://127.0.0.1:8081'
const password = 'Load-test-123456'
const runID = `${Date.now().toString(36)}${Math.floor(Math.random() * 100000)}`
const scenarioVUs = Number(__ENV.VUS || 10)
const scenarioDuration = __ENV.DURATION || '30s'
const sendIntervalSeconds = Number(__ENV.SLEEP_SECONDS || 0.8)

export const options = {
    setupTimeout: '5m',
    scenarios: {
        send_messages: {
            executor: 'constant-vus',
            vus: scenarioVUs,
            duration: scenarioDuration,
        },
    },
    thresholds: {
        checks: ['rate>0.99'],
        'http_req_failed{endpoint:send_message}': ['rate<0.01'],
        'http_req_duration{endpoint:send_message}': [
            'p(95)<1000',
            'p(99)<2000',
        ],
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

function retryAfterSeconds(response) {
    const retryAfter = Number(
        response.headers['Retry-After'] ||
        response.headers['Retry-after'] ||
        1,
    )
    return Math.max(1, retryAfter) + Math.random() * 0.25
}

function register(username, nickname) {
    for (let attempt = 0; attempt < 20; attempt += 1) {
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
        if (response.status === 429) {
            sleep(retryAfterSeconds(response))
            continue
        }
        requireStatus(response, 201, `注册 ${username}`)
        return response.json()
    }
    fail(`注册 ${username} 持续被限流`)
}

function login(username) {
    for (let attempt = 0; attempt < 20; attempt += 1) {
        const response = http.post(
            `${baseURL}/api/v1/auth/login`,
            JSON.stringify({username, password}),
            {headers: {'Content-Type': 'application/json'}},
        )
        if (response.status === 429) {
            sleep(retryAfterSeconds(response))
            continue
        }
        requireStatus(response, 200, `登录 ${username}`)
        return response.json()
    }
    fail(`登录 ${username} 持续被限流`)
}

export function setup() {
    const receiverUsername = `load_http_receiver_${runID}`
    const receiver = register(receiverUsername, '压测接收者')
    const receiverSession = login(receiverUsername)
    const senders = []

    for (let index = 0; index < scenarioVUs; index += 1) {
        const senderUsername = `load_http_sender_${runID}_${index}`
        const sender = register(senderUsername, `压测发送者${index + 1}`)
        const senderSession = login(senderUsername)

        const friendRequest = http.post(
            `${baseURL}/api/v1/friend-requests`,
            JSON.stringify({
                target_user_id: receiver.id,
                remark: 'k6 HTTP baseline',
            }),
            {headers: requestHeaders(senderSession.token)},
        )
        requireStatus(friendRequest, 201, '发送好友申请')

        const received = http.get(
            `${baseURL}/api/v1/friend-requests/received`,
            {headers: requestHeaders(receiverSession.token)},
        )
        requireStatus(received, 200, '查询好友申请')
        const request = received
            .json()
            .requests.find((item) => item.user.id === sender.id)
        if (!request) {
            fail('未找到压测好友申请')
        }
        requireStatus(
            http.post(
                `${baseURL}/api/v1/friend-requests/${request.id}/accept`,
                null,
                {headers: requestHeaders(receiverSession.token)},
            ),
            200,
            '同意好友申请',
        )

        const conversationResponse = http.post(
            `${baseURL}/api/v1/conversations/private`,
            JSON.stringify({peer_user_id: receiver.id}),
            {headers: requestHeaders(senderSession.token)},
        )
        requireStatus(conversationResponse, 201, '创建压测私聊')
        senders.push({
            username: senderUsername,
            token: senderSession.token,
            conversationID: conversationResponse.json().id,
        })
    }

    return {
        runID,
        receiverUsername,
        senders,
    }
}

export default function (data) {
    const sender = data.senders[(__VU - 1) % data.senders.length]
    const clientMessageID =
        `load-${data.runID}-${__VU}-${__ITER}-${Date.now()}`
    const response = http.post(
        `${baseURL}/api/v1/conversations/${sender.conversationID}/messages`,
        JSON.stringify({
            client_message_id: clientMessageID,
            content: `k6 HTTP 基线消息 vu=${__VU} iter=${__ITER}`,
        }),
        {
            headers: requestHeaders(sender.token),
            tags: {endpoint: 'send_message'},
        },
    )
    check(response, {
        '消息 Accepted': (result) => result.status === 201,
        '返回服务端序号': (result) => {
            if (result.status !== 201) return false
            return Number(result.json().seq) > 0
        },
    })
    sleep(sendIntervalSeconds)
}

export function handleSummary(data) {
    const metricNames = [
        'checks',
        'http_req_failed',
        'http_req_failed{endpoint:send_message}',
        'http_reqs',
        'http_reqs{endpoint:send_message}',
        'iterations',
        'http_req_duration',
        'http_req_duration{endpoint:send_message}',
        'data_received',
        'data_sent',
    ]
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
            executor: 'constant-vus',
            vus: scenarioVUs,
            duration: scenarioDuration,
            sleepSeconds: sendIntervalSeconds,
            senderUsers: scenarioVUs,
        },
        metrics,
    }
    const outputPath =
        `loadtest/reports/${__ENV.SUMMARY_FILE || 'http-baseline.json'}`
    return {
        [outputPath]: JSON.stringify(payload, null, 2),
    }
}
