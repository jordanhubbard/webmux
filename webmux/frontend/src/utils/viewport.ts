import { useEffect, useState } from 'react';
import type { RefObject } from 'react';

export interface ViewportSize {
  width: number;
  height: number;
}

function browserViewportSize(): ViewportSize {
  if (typeof window === 'undefined') return { width: 0, height: 0 };
  const viewport = window.visualViewport;
  return {
    width: Math.floor(viewport?.width ?? window.innerWidth ?? 0),
    height: Math.floor(viewport?.height ?? window.innerHeight ?? 0),
  };
}

export function isTouchLikeViewport(): boolean {
  if (typeof window === 'undefined') return false;
  return Boolean(
    window.matchMedia?.('(pointer: coarse)').matches ||
    window.navigator.maxTouchPoints > 0,
  );
}

export function useVisualViewportSize(): ViewportSize {
  const [size, setSize] = useState<ViewportSize>(() => browserViewportSize());

  useEffect(() => {
    const update = () => setSize(browserViewportSize());
    const viewport = window.visualViewport;
    window.addEventListener('resize', update);
    window.addEventListener('orientationchange', update);
    viewport?.addEventListener('resize', update);
    viewport?.addEventListener('scroll', update);
    update();
    return () => {
      window.removeEventListener('resize', update);
      window.removeEventListener('orientationchange', update);
      viewport?.removeEventListener('resize', update);
      viewport?.removeEventListener('scroll', update);
    };
  }, []);

  return size;
}

export function useTouchLikeViewport(): boolean {
  const [touchLike, setTouchLike] = useState(() => isTouchLikeViewport());

  useEffect(() => {
    const media = window.matchMedia?.('(pointer: coarse)');
    const update = () => setTouchLike(isTouchLikeViewport());
    if (media?.addEventListener) {
      media.addEventListener('change', update);
    } else {
      media?.addListener?.(update);
    }
    window.addEventListener('resize', update);
    update();
    return () => {
      if (media?.removeEventListener) {
        media.removeEventListener('change', update);
      } else {
        media?.removeListener?.(update);
      }
      window.removeEventListener('resize', update);
    };
  }, []);

  return touchLike;
}

export function useElementSize<T extends HTMLElement>(ref: RefObject<T | null>): ViewportSize {
  const [size, setSize] = useState<ViewportSize>({ width: 0, height: 0 });

  useEffect(() => {
    const element = ref.current;
    if (!element) return;

    const update = () => {
      const rect = element.getBoundingClientRect();
      setSize({
        width: Math.floor(rect.width),
        height: Math.floor(rect.height),
      });
    };

    update();
    const observer = typeof ResizeObserver !== 'undefined' ? new ResizeObserver(update) : null;
    observer?.observe(element);
    window.addEventListener('resize', update);
    window.visualViewport?.addEventListener('resize', update);
    return () => {
      observer?.disconnect();
      window.removeEventListener('resize', update);
      window.visualViewport?.removeEventListener('resize', update);
    };
  }, [ref]);

  return size;
}
