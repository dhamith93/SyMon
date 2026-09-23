import { describe, expect, it } from 'vitest';
import { align, maxSeries } from './align';

describe('align', () => {
  it('puts series on one time axis', () => {
    const aligned = align([
      { label: 'a', points: [[10, 1], [20, 2]] },
      { label: 'b', points: [[20, 5], [30, 6]] },
    ]);
    expect(aligned.times).toEqual([10, 20, 30]);
    expect(aligned.values).toEqual([
      [1, 2, undefined],
      [undefined, 5, 6],
    ]);
  });

  it('breaks lines at outages', () => {
    const aligned = align([{ label: 'cpu', points: [[0, 1], [15, 1], [30, 1], [45, 1], [300, 1], [315, 1]] }]);
    // one added time, halfway into the 45s to 300s gap, with a null
    expect(aligned.times).toEqual([0, 15, 30, 45, 172.5, 300, 315]);
    expect(aligned.values[0][4]).toBeNull();
  });

  it('keeps at most 8 series and says which were left out', () => {
    const series = Array.from({ length: maxSeries + 2 }, (_, i) => ({ label: `disk${i}`, points: [[0, i]] as [number, number][] }));
    const aligned = align(series);
    expect(aligned.labels).toHaveLength(maxSeries);
    expect(aligned.hidden).toEqual(['disk8', 'disk9']);
  });
});
