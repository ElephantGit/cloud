import { type ReactNode, useState } from 'react'
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from '@/lib/workflows/host/resizable'

const widthKey = 'ora.v2.workspace-panel.width'
const defaultWidth = 480
/** Bounds of the workspace panel, and the narrowest the page beside it may get. */
const minWidth = 300
const minMainWidth = 360

function rememberedWidth(): number {
  try {
    const n = Number(localStorage.getItem(widthKey))
    return Number.isFinite(n) && n >= minWidth ? n : defaultWidth
  } catch {
    return defaultWidth
  }
}

/**
 * A page's main area with the workspace panel beside it on a resizable split, the same resizable
 * the workflow editor uses. The panel keeps its width in pixels when the window resizes, and the
 * width the person drags to is remembered per browser.
 */
export function WorkspaceSplit({ id, panel, children }: { id: string; panel: ReactNode | null; children: ReactNode }) {
  const [width] = useState(rememberedWidth)
  return (
    <ResizablePanelGroup orientation="horizontal" className="min-h-0 min-w-0 flex-1" resizeTargetMinimumSize={{ coarse: 28, fine: 12 }}>
      <ResizablePanel id={`${id}-main`} minSize={minMainWidth}>
        {children}
      </ResizablePanel>
      {panel && (
        <>
          <ResizableHandle withHandle aria-label="调整工作区宽度" className="z-20 transition-colors after:w-3 hover:bg-ring focus-visible:bg-ring" />
          <ResizablePanel
            id={`${id}-workspace`}
            defaultSize={width}
            minSize={minWidth}
            maxSize="75%"
            groupResizeBehavior="preserve-pixel-size"
            onResize={(size) => {
              try {
                if (size.inPixels >= minWidth) localStorage.setItem(widthKey, String(Math.round(size.inPixels)))
              } catch {
                // Blocked storage only forgets the width.
              }
            }}
          >
            {panel}
          </ResizablePanel>
        </>
      )}
    </ResizablePanelGroup>
  )
}
