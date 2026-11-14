import http from 'k6/http'
import {check, sleep} from 'k6'
import {Counter} from 'k6/metrics'

const baseURL = __ENV.BASE_URL || 'http://127.0.0.1:8081'
const fixtureFile =
    __ENV.FIXTURE_FILE || '../../.tmp/group-capacity-fixture.json'
const fixture = JSON.parse(open(fixtureFile))
const scenarioVUs = Number(__ENV.VUS || 75)
const scenarioDuration = __ENV.DURATION || '30s'
const sendIntervalSeconds = Number(__ENV.SLEEP_SECONDS || 0.8)
const rateLimited = new Counter('qps_send_rate_limited')

export const options = {
    scenarios: {
        send_messages: {
            executor: 'constant-vus',
            vus: scenarioVUs,
            duration: scenarioDuration,
        },
    },
    thresholds: {
        checks: ['rate>0.99'],
        'http_req_failed{endpoint:qps_send}': ['rate<0.01'],
        'http_req_duration{endpoint:qps_send}': [
            'p(95)<1000',
            'p(99)<2000',
        ],
    },
    summaryTrendStats: ['avg', 'min', 'med', 'p(90)', 'p(95)', 'p(99)', 'max'],
}

// requestHeaders 生成消息发送请求头
function requestHeaders(token) {
    return {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${token}`,
    }
}

export default function () {
    const memberIndex = (__VU - 1) % fixture.members.length
    const member = fixture.members[memberIndex]
    const clientMessageID =
        `qps-${fixture.run_id}-${__VU}-${__ITER}-${Date.now()}`
    const response = http.post(
        `${baseURL}/api/v1/conversations/${fixture.conversation_id}/messages`,
        JSON.stringify({
            client_message_id: clientMessageID,
            message_type: 1,
            content: `QPS 容量测试 vu=${__VU} iter=${__ITER}`,
        }),
        {
            headers: requestHeaders(member.token),
            tags: {endpoint: 'qps_send'},
        },
    )
    if (response.status === 429) {
        rateLimited.add(1)
    }
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
        'http_req_failed{endpoint:qps_send}',
        'http_reqs',
        'http_reqs{endpoint:qps_send}',
        'iterations',
        'http_req_duration',
        'http_req_duration{endpoint:qps_send}',
        'qps_send_rate_limited',
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
            targetAttemptQPS: scenarioVUs / sendIntervalSeconds,
            senderUsers: Math.min(scenarioVUs, fixture.members.length),
        },
        fixture: {
            memberCount: fixture.members.length,
            conversationType: 'group',
        },
        metrics,
    }
    return {
        [`loadtest/reports/${__ENV.SUMMARY_FILE || 'http-qps.json'}`]:
            JSON.stringify(payload, null, 2),
    }
}
