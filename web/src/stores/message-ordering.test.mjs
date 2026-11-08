import assert from 'node:assert/strict'
import test from 'node:test'

import {
    createSequenceCursor,
    establishSequenceBaseline,
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
