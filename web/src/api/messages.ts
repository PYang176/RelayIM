import {request} from './http'
import type {
    AttachmentResponse,
    DeliverySummary,
    MessagePage,
    ReadState,
    SendMessageResponse,
} from '@/types/api'

export function listMessages(
    token: string,
    conversationId: string,
    query: { afterSeq?: number; beforeSeq?: number; limit?: number },
): Promise<MessagePage> {
    const params = new URLSearchParams()
    if (query.afterSeq) params.set('after_seq', String(query.afterSeq))
    if (query.beforeSeq) params.set('before_seq', String(query.beforeSeq))
    params.set('limit', String(query.limit ?? 30))
    return request(`/conversations/${conversationId}/messages?${params}`, {}, token)
}

export function sendText(
    token: string,
    conversationId: string,
    clientMessageId: string,
    content: string,
): Promise<SendMessageResponse> {
    return request(
        `/conversations/${conversationId}/messages`,
        {
            method: 'POST',
            body: JSON.stringify({
                client_message_id: clientMessageId,
                content,
            }),
        },
        token,
    )
}

export function sendAttachmentMessage(
    token: string,
    conversationId: string,
    clientMessageId: string,
    messageType: number,
    attachmentId: string,
    content = '',
): Promise<SendMessageResponse> {
    return request(
        `/conversations/${conversationId}/messages`,
        {
            method: 'POST',
            body: JSON.stringify({
                client_message_id: clientMessageId,
                message_type: messageType,
                attachment_id: attachmentId,
                content,
            }),
        },
        token,
    )
}

export function uploadAttachment(
    token: string,
    file: File,
    onProgress: (progress: number) => void,
): Promise<AttachmentResponse> {
    return new Promise((resolve, reject) => {
        const request = new XMLHttpRequest()
        request.open('POST', '/api/v1/attachments')
        request.setRequestHeader('Authorization', `Bearer ${token}`)
        request.upload.onprogress = (event) => {
            if (event.lengthComputable) {
                onProgress(Math.round((event.loaded / event.total) * 100))
            }
        }
        request.onerror = () => reject(new Error('上传附件失败'))
        request.onload = () => {
            let body: unknown
            try {
                body = JSON.parse(request.responseText)
            } catch {
                body = null
            }
            if (request.status < 200 || request.status >= 300) {
                const message =
                    body &&
                    typeof body === 'object' &&
                    'error' in body &&
                    typeof body.error === 'string'
                        ? body.error
                        : `上传附件失败（${request.status}）`
                reject(new Error(message))
                return
            }
            resolve(body as AttachmentResponse)
        }
        const data = new FormData()
        data.append('file', file)
        request.send(data)
    })
}

export function getAttachmentInfo(
    token: string,
    attachmentId: string,
): Promise<AttachmentResponse> {
    return request(`/attachments/${attachmentId}`, {}, token)
}

export async function listReadStates(
    token: string,
    conversationId: string,
): Promise<ReadState[]> {
    const response = await request<{ read_states: ReadState[] }>(
        `/conversations/${conversationId}/read-states`,
        {},
        token,
    )
    return response.read_states
}

export function getDeliverySummary(
    token: string,
    conversationId: string,
    messageId: string,
): Promise<DeliverySummary> {
    return request(
        `/conversations/${conversationId}/messages/${messageId}/delivery`,
        {},
        token,
    )
}
