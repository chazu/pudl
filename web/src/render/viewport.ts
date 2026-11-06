// Pan/zoom math. Pure functions; the canvas owns the state.

import type { Box, Point } from '../layout/types';

export interface Viewport {
  x: number;
  y: number;
  k: number;
}

export const MIN_ZOOM = 0.02;
export const MAX_ZOOM = 3;

const clamp = (k: number) => Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, k));

/** Zoom by `factor` keeping the screen point `at` fixed. */
export function zoomAt(vp: Viewport, factor: number, at: Point): Viewport {
  const k = clamp(vp.k * factor);
  const f = k / vp.k;
  return { k, x: at.x - (at.x - vp.x) * f, y: at.y - (at.y - vp.y) * f };
}

export function fitBox(box: Box, width: number, height: number, padding = 32): Viewport {
  if (box.width <= 0 || box.height <= 0 || width <= 0 || height <= 0) return { x: padding, y: padding, k: 1 };
  const k = clamp(Math.min((width - padding * 2) / box.width, (height - padding * 2) / box.height, 1.2));
  return {
    k,
    x: (width - box.width * k) / 2 - box.x * k,
    y: (height - box.height * k) / 2 - box.y * k,
  };
}

/** Zoom at which node labels are comfortably legible. */
export const READABLE_ZOOM = 0.8;

/**
 * Center `box` on screen. With `minZoom`, also zoom in to at least that
 * level (never out), as far as the box still fits on screen.
 */
export function centerOn(vp: Viewport, box: Box, width: number, height: number, minZoom?: number, padding = 32): Viewport {
  let k = vp.k;
  if (minZoom !== undefined && k < minZoom) {
    const fits = Math.min((width - padding * 2) / Math.max(box.width, 1), (height - padding * 2) / Math.max(box.height, 1));
    k = clamp(Math.max(k, Math.min(minZoom, fits)));
  }
  const cx = box.x + box.width / 2;
  const cy = box.y + box.height / 2;
  return { k, x: width / 2 - cx * k, y: height / 2 - cy * k };
}

/**
 * Keep an anchor fixed on screen across a relayout: shift the viewport by
 * the anchor's movement in graph space.
 */
export function compensate(vp: Viewport, before: Box, after: Box): Viewport {
  return { k: vp.k, x: vp.x - (after.x - before.x) * vp.k, y: vp.y - (after.y - before.y) * vp.k };
}
