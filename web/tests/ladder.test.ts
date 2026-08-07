import { describe, expect, it } from 'vitest'
import { timeLadder } from '../src/lib/transform/ladder'
import type { TimelineTotals } from '../src/lib/types'

/** The reference session's real numbers, from `docs/api.md` § The ladder (verified 2026-08-08). */
const reference: Pick<TimelineTotals, 'wallClockSeconds' | 'laneTimeSeconds' | 'netSeconds' | 'activeSeconds'> = {
    wallClockSeconds: 276792.472,
    laneTimeSeconds: 428756.432,
    netSeconds: 162664.28,
    activeSeconds: 138018.558,
}

describe('timeLadder', () => {
    it('puts the three rungs in order, widest first, so each one reads as the one above minus something', () => {
        expect(timeLadder(reference).rungs.map((r) => r.key)).toEqual(['lane', 'net', 'active'])
    })

    it('names what each rung lost, which is the only thing that stops net being quoted as lane time', () => {
        const [lane, net, active] = timeLadder(reference).rungs
        expect(lane.subtracted).toBe(0)
        expect(net.subtracted).toBeCloseTo(428756.432 - 162664.28, 3)
        expect(active.subtracted).toBeCloseTo(162664.28 - 138018.558, 3)
    })

    it('shares are of lane time, so the rungs nest inside the bar above them', () => {
        const [lane, net, active] = timeLadder(reference).rungs
        expect(lane.share).toBe(1)
        expect(net.share).toBeCloseTo(162664.28 / 428756.432, 6)
        expect(active.share).toBeCloseTo(138018.558 / 428756.432, 6)
        expect(net.share).toBeGreaterThan(active.share)
    })

    it('keeps wall clock beside the ladder rather than as a rung, because it is not lane time minus anything', () => {
        const ladder = timeLadder(reference)
        expect(ladder.rungs.map((r) => r.seconds)).not.toContain(reference.wallClockSeconds)
        expect(ladder.wallClockSeconds).toBe(276792.472)
        // Lane time is the larger of the two whenever lanes ran at once, which is the whole point of both.
        expect(ladder.laneSeconds).toBeGreaterThan(ladder.wallClockSeconds)
    })

    it('a session that never waited has net equal to lane time, and subtracts nothing to say so', () => {
        const ladder = timeLadder({
            wallClockSeconds: 100,
            laneTimeSeconds: 100,
            netSeconds: 100,
            activeSeconds: 90,
        })
        const [, net, active] = ladder.rungs
        expect(net.subtracted).toBe(0)
        expect(net.share).toBe(1)
        expect(active.subtracted).toBeCloseTo(10, 6)
    })

    it('survives a session with no derivable time rather than dividing by zero', () => {
        const ladder = timeLadder({
            wallClockSeconds: 0,
            laneTimeSeconds: 0,
            netSeconds: 0,
            activeSeconds: 0,
        })
        expect(ladder.rungs.map((r) => r.share)).toEqual([0, 0, 0])
        expect(ladder.rungs.map((r) => r.subtracted)).toEqual([0, 0, 0])
    })

    it('clamps a rung that arrives larger than the one above it instead of drawing past the bar', () => {
        // Can't happen from the engine; a clamp here means a bad response degrades to a readable page.
        const ladder = timeLadder({
            wallClockSeconds: 10,
            laneTimeSeconds: 100,
            netSeconds: 120,
            activeSeconds: 130,
        })
        expect(ladder.rungs.map((r) => r.share)).toEqual([1, 1, 1])
        expect(ladder.rungs.map((r) => r.subtracted)).toEqual([0, 0, 0])
    })
})
