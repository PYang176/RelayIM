export interface SequenceCursor {
    contiguousSeq: number
    pendingSeqs: Set<number>
}

export interface SequenceObservation {
    advanced: boolean
    gapDetected: boolean
}

// needsLatestWindowSync 判断最新窗口是否需要整体重同步
export function needsLatestWindowSync(
    hasWindow: boolean,
    serverLastSeq: number,
    contiguousSeq: number,
): boolean {
    return !hasWindow || serverLastSeq > contiguousSeq
}

// createSequenceCursor 创建会话连续序号游标
export function createSequenceCursor(contiguousSeq = 0): SequenceCursor {
    return {
        contiguousSeq: Math.max(0, contiguousSeq),
        pendingSeqs: new Set(),
    }
}

// observeSequence 观察消息序号并计算连续水位和缺口
export function observeSequence(
    cursor: SequenceCursor,
    seq: number,
): SequenceObservation {
    if (!Number.isSafeInteger(seq) || seq <= 0) {
        return {
            advanced: false,
            gapDetected: cursor.pendingSeqs.size > 0,
        }
    }
    if (seq <= cursor.contiguousSeq) {
        cursor.pendingSeqs.delete(seq)
        return {
            advanced: false,
            gapDetected: cursor.pendingSeqs.size > 0,
        }
    }
    if (seq > cursor.contiguousSeq + 1) {
        cursor.pendingSeqs.add(seq)
        return {
            advanced: false,
            gapDetected: true,
        }
    }

    cursor.contiguousSeq = seq
    cursor.pendingSeqs.delete(seq)
    while (cursor.pendingSeqs.has(cursor.contiguousSeq + 1)) {
        cursor.contiguousSeq += 1
        cursor.pendingSeqs.delete(cursor.contiguousSeq)
    }
    return {
        advanced: true,
        gapDetected: cursor.pendingSeqs.size > 0,
    }
}

// establishSequenceBaseline 根据历史窗口建立新的连续序号基线
export function establishSequenceBaseline(
    cursor: SequenceCursor,
    contiguousSeq: number,
): SequenceObservation {
    if (!Number.isSafeInteger(contiguousSeq) || contiguousSeq <= 0) {
        return {
            advanced: false,
            gapDetected: cursor.pendingSeqs.size > 0,
        }
    }
    const advanced = contiguousSeq > cursor.contiguousSeq
    if (advanced) {
        cursor.contiguousSeq = contiguousSeq
    }
    for (const pendingSeq of cursor.pendingSeqs) {
        if (pendingSeq <= cursor.contiguousSeq) {
            cursor.pendingSeqs.delete(pendingSeq)
        }
    }
    while (cursor.pendingSeqs.has(cursor.contiguousSeq + 1)) {
        cursor.contiguousSeq += 1
        cursor.pendingSeqs.delete(cursor.contiguousSeq)
    }
    return {
        advanced,
        gapDetected: cursor.pendingSeqs.size > 0,
    }
}
