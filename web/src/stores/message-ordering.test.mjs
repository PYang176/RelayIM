import assert from 'node:assert/strict'
import test from 'node:test'

import {
    createSequenceCursor,
    establishSequenceBaseline,
    needsLatestWindowSync,
    observeSequence,
} from './message-ordering.ts'

test('第三条先到时记录缺口，前两条补到后连续推进', () => {
    const cursor = createSequenceCursor()

    assert.deepEqual(observeSequence(cursor, 3), {
        advanced: false,
        gapDetected: true,
    })
    assert.equal(cursor.contiguousSeq, 0)
    assert.deepEqual([...cursor.pendingSeqs], [3])

    assert.deepEqual(observeSequence(cursor, 1), {
        advanced: true,
        gapDetected: true,
    })
    assert.equal(cursor.contiguousSeq, 1)

    assert.deepEqual(observeSequence(cursor, 2), {
        advanced: true,
        gapDetected: false,
    })
    assert.equal(cursor.contiguousSeq, 3)
    assert.equal(cursor.pendingSeqs.size, 0)
})

test('重复和晚到消息不会倒退连续水位', () => {
    const cursor = createSequenceCursor()
    observeSequence(cursor, 1)
    observeSequence(cursor, 2)

    assert.deepEqual(observeSequence(cursor, 2), {
        advanced: false,
        gapDetected: false,
    })
    assert.equal(cursor.contiguousSeq, 2)
})

test('历史页建立基线后继续检测后续缺口', () => {
    const cursor = createSequenceCursor()
    establishSequenceBaseline(cursor, 140)

    assert.deepEqual(observeSequence(cursor, 143), {
        advanced: false,
        gapDetected: true,
    })
    assert.equal(cursor.contiguousSeq, 140)

    observeSequence(cursor, 141)
    observeSequence(cursor, 142)
    assert.equal(cursor.contiguousSeq, 143)
    assert.equal(cursor.pendingSeqs.size, 0)
})

test('离线差距较大时选择最新窗口而不是连续拉取全部历史', () => {
    assert.equal(needsLatestWindowSync(true, 500, 80), true)
    assert.equal(needsLatestWindowSync(true, 500, 500), false)
    assert.equal(needsLatestWindowSync(false, 0, 0), true)
})

test('最新页建立新基线后清除旧窗口缺口', () => {
    const cursor = createSequenceCursor(80)
    observeSequence(cursor, 82)

    assert.deepEqual(establishSequenceBaseline(cursor, 500), {
        advanced: true,
        gapDetected: false,
    })
    assert.equal(cursor.contiguousSeq, 500)
    assert.equal(cursor.pendingSeqs.size, 0)
    assert.equal(needsLatestWindowSync(true, 500, cursor.contiguousSeq), false)
})

test('实时窗口缺口通过 after_seq 分页反复补齐', () => {
    const cursor = createSequenceCursor()
    establishSequenceBaseline(cursor, 500)
    assert.deepEqual(observeSequence(cursor, 503), {
        advanced: false,
        gapDetected: true,
    })

    for (const seq of [501, 502, 503]) {
        observeSequence(cursor, seq)
    }
    assert.equal(cursor.contiguousSeq, 503)
    assert.equal(cursor.pendingSeqs.size, 0)
})
