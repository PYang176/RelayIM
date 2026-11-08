import http from 'k6/http'
import ws from 'k6/ws'
import {check, fail, sleep} from 'k6'
import {Counter, Rate, Trend} from 'k6/metrics'

const baseURL = __ENV.BASE_URL || 'http://127.0.0.1:8081'
const wsURL = baseURL.replace(/^http/, 'ws')
const password = 'Load-test-123456'
const runID = `${Date.now().toString(36)}${Math.floor(Math.random() * 100000)}`
const connectionCount = Number(__ENV.WS_CONNECTIONS || 100)
const connectionsPerUser = 10
const userCount = Math.ceil(connectionCount / connectionsPerUser)
const holdSeconds = Number(__ENV.HOLD_SECONDS || 20)
const connectionRampSeconds = Number(__ENV.CONNECT_RAMP_SECONDS || 5)
const scenarioTailSeconds = 5

const wsConnectionSuccess = new Rate('ws_connection_success')
const wsUnexpectedClose = new Counter('ws_unexpected_close')
const wsConnectDuration = new Trend('ws_connect_duration')

export const options = {
    setupTimeout: '3m',
    scenarios: {
        connections: {
            executor: 'ramping-vus',
            exec: 'holdConnection',
            startVUs: 0,
            stages: [
                {
                    duration: `${connectionRampSeconds}s`,
                    target: connectionCount,
                },
                {
                    duration: `${holdSeconds + scenarioTailSeconds}s`,
                    target: connectionCount,
                },
            ],
            gracefulRampDown: '5s',
        },
    },
    thresholds: {
        checks: ['rate>0.99'],
        ws_connection_success: ['rate>0.99'],
        ws_unexpected_close: ['count==0'],
        ws_connect_duration: ['p(95)<1000'],
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

function requestWSTicket(token) {
    const maxAttempts = Number(__ENV.TICKET_RETRY_ATTEMPTS || 120)
    for (let attempt = 0; attempt < maxAttempts; attempt += 1) {
        const response = http.post(
            `${baseURL}/api/v1/auth/ws-ticket`,
            JSON.stringify({
                device_name: `k6-connections-${__VU}`,
                platform: 1,
            }),
            {headers: requestHeaders(token)},
        )
        if (response.status === 200) {
            return response.json().ticket
        }
        if (response.status !== 429) {
            fail(
                `申请连接基线 WebSocket Ticket失败: ` +
                `status=${response.status} body=${response.body}`,
            )
        }
        const retryAfter = Number(
            response.headers['Retry-After'] ||
            response.headers['Retry-after'] ||
            1,
        )
        sleep(retryAfter + Math.random() * 0.25)
    }
    fail('申请连接基线 WebSocket Ticket持续被限流')
}

export function setup() {
    const users = []
    for (let index = 0; index < userCount; index += 1) {
        const username = `loadconn_${runID}_${index}`
        register(username, `连接测试用户${index + 1}`)
        const session = login(username)
        users.push({token: session.token})
    }
    // setup 返回后场景时间固定，所有连接持有到该绝对时间，
    // 保证完成爬坡后至少 holdSeconds 内连接数是完整的目标值。
    const holdUntil =
        Date.now() +
        (connectionRampSeconds + holdSeconds + 1) * 1000
    return {runID, users, holdUntil}
}

export function holdConnection(data) {
    if (Date.now() >= data.holdUntil) {
        sleep(1)
        return
    }

    const userIndex =
        Math.floor((__VU - 1) / connectionsPerUser) % data.users.length
    const user = data.users[userIndex]
    const ticket = requestWSTicket(user.token)
    const connectStartedAt = Date.now()
    let opened = false
    let expectedClose = false

    const response = ws.connect(
        `${wsURL}/ws?ticket=${encodeURIComponent(ticket)}`,
        {tags: {endpoint: 'ws_connect_scale'}},
        function (socket) {
            socket.on('open', () => {
                opened = true
                wsConnectDuration.add(Date.now() - connectStartedAt)
                wsConnectionSuccess.add(true)
            })
            socket.on('close', () => {
                if (opened && !expectedClose) {
                    wsUnexpectedClose.add(1)
                }
            })
            socket.on('error', () => {
                if (!opened) {
                    wsConnectionSuccess.add(false)
                }
            })
            socket.setTimeout(() => {
                expectedClose = true
                socket.close()
            }, Math.max(100, data.holdUntil - Date.now()))
        },
    )
    check(response, {
        'WebSocket 握手成功': (result) => result && result.status === 101,
    })
    if (!opened) {
        wsConnectionSuccess.add(false)
    }
}

export function handleSummary(data) {
    const requiredMetrics = [
        'checks',
        'http_req_failed',
        'http_reqs',
        'ws_connection_success',
        'ws_unexpected_close',
        'ws_connect_duration',
        'ws_sessions',
        'vus_max',
    ]
    const metricNames = Object.keys(data.metrics).filter(
        (name) =>
            requiredMetrics.includes(name) ||
            name.startsWith('ws_'),
    )
    const metrics = {}
    for (const name of metricNames) {
        const metric = data.metrics[name]
        metrics[name] = {
            type: metric.type,
            values: metric.values,
            thresholds: metric.thresholds || {},
        }
    }
    const outputPath =
        `loadtest/reports/${__ENV.SUMMARY_FILE || 'websocket-connections.json'}`
    return {
        [outputPath]: JSON.stringify({
            scenario: {
                connectionCount,
                connectionsPerUser,
                userCount,
                holdSeconds,
                connectionRampSeconds,
                scenarioTailSeconds,
            },
            metrics,
        }, null, 2),
    }
}
