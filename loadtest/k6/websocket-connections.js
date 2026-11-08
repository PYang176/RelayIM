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

const wsConnectionSuccess = new Rate('ws_connection_success')
const wsUnexpectedClose = new Counter('ws_unexpected_close')
const wsConnectDuration = new Trend('ws_connect_duration')

export const options = {
    setupTimeout: '2m',
    scenarios: {
        connections: {
            executor: 'per-vu-iterations',
            exec: 'holdConnection',
            vus: connectionCount,
            iterations: 1,
            maxDuration: `${holdSeconds + 10}s`,
            gracefulStop: '10s',
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

export function setup() {
    const users = []
    for (let index = 0; index < userCount; index += 1) {
        const username = `loadconn_${runID}_${index}`
        register(username, `连接测试用户${index + 1}`)
        const session = login(username)
        users.push({token: session.token})
    }
    return {runID, users}
}

export function holdConnection(data) {
    sleep(Math.random() * 5)

    const userIndex =
        Math.floor((__VU - 1) / connectionsPerUser) % data.users.length
    const user = data.users[userIndex]
    const ticketResponse = http.post(
        `${baseURL}/api/v1/auth/ws-ticket`,
        JSON.stringify({
            device_name: `k6-connections-${__VU}`,
            platform: 1,
        }),
        {headers: requestHeaders(user.token)},
    )
    requireStatus(ticketResponse, 200, '申请连接基线 WebSocket Ticket')
    const ticket = ticketResponse.json().ticket
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
            }, holdSeconds * 1000)
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
            },
            metrics,
        }, null, 2),
    }
}
