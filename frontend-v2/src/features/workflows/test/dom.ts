// The jsdom gaps desktop's test setup fills for the editor (React Flow, ProseMirror, Base UI), kept
// with the workflow tests instead of the shared setup. The ResizeObserver reports the observed
// box so geometry-driven code does not derive NaN coordinates.

// jsdom lacks matchMedia; settings theme subscription depends on it.
if (!window.matchMedia) {
  window.matchMedia = (query: string): MediaQueryList =>
    ({
      matches: false,
      media: query,
      onchange: null,
      addEventListener: () => {},
      removeEventListener: () => {},
      addListener: () => {},
      removeListener: () => {},
      dispatchEvent: () => false,
    }) as MediaQueryList;
}

// The shared setup installs a silent ResizeObserver; replace it with one that reports the
// fixture's declared dimensions.
{
  globalThis.ResizeObserver = class {
    private readonly callback: ResizeObserverCallback;

    constructor(callback: ResizeObserverCallback) {
      this.callback = callback;
    }

    observe(target: Element) {
      const bounds = target.getBoundingClientRect();
      const width =
        bounds.width ||
        (target instanceof HTMLElement ? target.clientWidth : 0);
      const height =
        bounds.height ||
        (target instanceof HTMLElement ? target.clientHeight : 0);
      const contentRect = {
        x: bounds.x,
        y: bounds.y,
        top: bounds.top,
        right: bounds.left + width,
        bottom: bounds.top + height,
        left: bounds.left,
        width,
        height,
        toJSON: () => ({}),
      };
      this.callback(
        [
          {
            target,
            contentRect,
            borderBoxSize: [],
            contentBoxSize: [],
            devicePixelContentBoxSize: [],
          },
        ],
        this,
      );
    }

    unobserve() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver;
}

// React Flow reads the viewport's vertical scale while processing observed
// node dimensions. jsdom does not provide DOMMatrixReadOnly.
if (!window.DOMMatrixReadOnly) {
  Object.defineProperty(window, "DOMMatrixReadOnly", {
    configurable: true,
    value: class {
      readonly m22: number;

      constructor(transform = "") {
        const scale = /scale\(([^)]+)\)/u.exec(transform)?.[1];
        this.m22 = scale === undefined ? 1 : Number(scale);
      }
    },
  });
}

// jsdom does not implement scrollIntoView; cmdk scrolls its active item into
// view whenever the selection changes.
if (!Element.prototype.scrollIntoView) {
  Element.prototype.scrollIntoView = () => {};
}

// ProseMirror maps pointer coordinates through document.elementFromPoint.
// jsdom leaves it undefined, which throws on mousedown and drops typed input.
if (typeof document.elementFromPoint !== "function") {
  document.elementFromPoint = () => null;
}
if (typeof document.caretRangeFromPoint !== "function") {
  document.caretRangeFromPoint = () => null;
}

const emptyClientRect = (): DOMRect =>
  ({
    x: 0,
    y: 0,
    width: 0,
    height: 0,
    top: 0,
    right: 0,
    bottom: 0,
    left: 0,
    toJSON: () => ({}),
  }) as DOMRect;

const emptyClientRectList = (): DOMRectList => {
  const list = [] as unknown as DOMRectList;
  Object.defineProperty(list, "item", {
    value: () => emptyClientRect(),
  });
  return list;
};

if (typeof Range !== "undefined") {
  if (typeof Range.prototype.getClientRects !== "function") {
    Range.prototype.getClientRects = emptyClientRectList;
  }
  if (typeof Range.prototype.getBoundingClientRect !== "function") {
    Range.prototype.getBoundingClientRect = emptyClientRect;
  }
}
if (typeof Text !== "undefined") {
  const textProto = Text.prototype as Text & {
    getClientRects?: () => DOMRectList;
  };
  if (typeof textProto.getClientRects !== "function") {
    textProto.getClientRects = emptyClientRectList;
  }
}

// jsdom does not implement the Web Animations API; Base UI's ScrollArea checks
// it after mount before recalculating scrollbar geometry.
if (!Element.prototype.getAnimations) {
  Element.prototype.getAnimations = () => [];
}

