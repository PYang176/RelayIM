import http from 'k6/http'
import {check, fail, sleep} from 'k6'
import {Counter} from 'k6/metrics'

const baseURL = __ENV.BASE_URL || 'http://127.0.0.1:8081'
const password = 'Load-test-123456'
const runID = `${Date.now().toString(36)}${Math.floor(Math.random() * 100000)}`
const rejected = new Counter('ws_ticket_rate_limited')
const accepted = new Counter('ws_ticket_accepted')

export const options = {
    scenarios: {
        burst: {
            executor: 'shared-iterations',
            vus: 1,
            iterations: 1,
        },
    },
    thresholds: {
        checks: ['rate>0.99'],
        ws_ticket_rate_limited: ['count>0'],
    },
}

function headers(token) {
    return {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${token}`,
    }
}

function requireStatus(response, expected, action) {
    if (response.status !== expected) {
        fail(`${action}失败: status=${response.status} body=${response.body}`)
    }
    return response.json()
}

export function setup() {
    const username = `loadrltest_${runID}`
    requireStatus(
        http.post(
            `${baseURL}/api/v1/auth/register`,
            JSON.stringify({
                username,
                password,
                confirm_password: password,
                nickname: 'Ticket 限流测试',
            }),
            {headers: {'Content-Type': 'application/json'}},
        ),
        201,
        '注册限流测试用户',
    )
    const session = requireStatus(
        http.post(
            `${baseURL}/api/v1/auth/login`,
            JSON.stringify({username, password}),
            {headers: {'Content-Type': 'application/json'}},
        ),
        200,
        '登录限流测试用户',
    )
    return {token: session.token}
}

export default function (data) {
    const requests = Array.from({length: 20}, () => ({
        method: 'POST',
        url: `${baseURL}/api/v1/auth/ws-ticket`,
        body: JSON.stringify({
            device_name: 'k6-rate-limit',
            platform: 1,
        }),
        params: {headers: headers(data.token)},
    }))
    const responses = http.batch(requests)
    let acceptedCount = 0
    let rejectedCount = 0
    let unexpectedCount = 0
    for (const response of responses) {
        if (response.status === 200) acceptedCount += 1
        else if (response.status === 429) rejectedCount += 1
        else unexpectedCount += 1
    }
    accepted.add(acceptedCount)
    rejected.add(rejectedCount)
    check(responses, {
        '所有请求均被明确处理': () => unexpectedCount === 0,
        '出现 429 限流响应': () => rejectedCount > 0,
        '允许配额内请求': () => acceptedCount > 0,
    })

    sleep(1.2)
    const retry = http.post(
        `${baseURL}/api/v1/auth/ws-ticket`,
        JSON.stringify({
            device_name: 'k6-rate-limit-retry',
            platform: 1,
        }),
        {headers: headers(data.token)},
    )
    check(retry, {
        '等待窗口后恢复': (response) => response.status === 200,
    })
}

export function handleSummary(data) {
    const metrics = {}
    for (const name of [
        'checks',
        'http_req_failed',
        'ws_ticket_accepted',
        'ws_ticket_rate_limited',
    ]) {
        const metric = data.metrics[name]
        if (!metric) continue
        metrics[name] = {
            type: metric.type,
            values: metric.values,
            thresholds: metric.thresholds || {},
        }
    }
    const outputPath =
        `loadtest/reports/${__ENV.SUMMARY_FILE || 'ticket-rate-limit.json'}`
    return {
        [outputPath]: JSON.stringify({metrics}, null, 2),
    }
}
