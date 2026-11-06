import { describe, expect, it } from 'vitest';
import { centerOn, READABLE_ZOOM } from './viewport';

describe('centerOn', () => {
  const box = { x: 1000, y: 500, width: 200, height: 40 };

  it('keeps the zoom without a minimum', () => {
    expect(centerOn({ x: 0, y: 0, k: 0.1 }, box, 800, 600).k).toBe(0.1);
  });

  it('zooms in to the minimum and centres the box', () => {
    const vp = centerOn({ x: 0, y: 0, k: 0.1 }, box, 800, 600, READABLE_ZOOM);
    expect(vp.k).toBe(READABLE_ZOOM);
    expect(vp.x + (box.x + box.width / 2) * vp.k).toBeCloseTo(400);
    expect(vp.y + (box.y + box.height / 2) * vp.k).toBeCloseTo(300);
  });

  it('never zooms out, and stops where a large box still fits', () => {
    expect(centerOn({ x: 0, y: 0, k: 2 }, box, 800, 600, READABLE_ZOOM).k).toBe(2);
    const big = { x: 0, y: 0, width: 4000, height: 1000 };
    expect(centerOn({ x: 0, y: 0, k: 0.05 }, big, 800, 600, READABLE_ZOOM).k).toBeCloseTo((800 - 64) / 4000);
  });
});
