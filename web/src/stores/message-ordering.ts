export interface SequenceCursor {
    contiguousSeq: number
    pendingSeqs: Set<number>
}

export interface SequenceObservation {
    advanced: boolean
    gapDetected: boolean
}

export function needsLatestWindowSync(
    hasWindow: boolean,
    serverLastSeq: number,
    contiguousSeq: number,
): boolean {
    return !hasWindow || serverLastSeq > contiguousSeq
}

export function createSequenceCursor(contiguousSeq = 0): SequenceCursor {
    return {
        contiguousSeq: Math.max(0, contiguousSeq),
        pendingSeqs: new Set(),
    }
}

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
