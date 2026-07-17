"use client"

import { useEffect, useState, RefObject } from "react"
import { createPortal } from "react-dom"

// Renders dropdown content into document.body instead of inline. Absolutely
// positioned elements nested inside a <td> can paint behind sibling <tr>s in
// some browsers due to table layout's own stacking/painting rules, even
// though hit-testing still targets them correctly - a portal sidesteps that
// entirely by escaping the table's DOM subtree.
export function DropdownPortal({
  anchorRef,
  children,
}: {
  anchorRef: RefObject<HTMLElement | null>
  children: React.ReactNode
}) {
  const [rect, setRect] = useState<{ top: number; left: number; width: number } | null>(null)

  useEffect(() => {
    function updateRect() {
      const el = anchorRef.current
      if (!el) return
      const r = el.getBoundingClientRect()
      setRect({ top: r.bottom, left: r.left, width: r.width })
    }
    updateRect()
    window.addEventListener("scroll", updateRect, true)
    window.addEventListener("resize", updateRect)
    return () => {
      window.removeEventListener("scroll", updateRect, true)
      window.removeEventListener("resize", updateRect)
    }
  }, [anchorRef])

  if (!rect) return null

  return createPortal(
    <div
      style={{ position: "fixed", top: rect.top, left: rect.left, width: rect.width, zIndex: 50 }}
      className="max-h-56 min-w-48 overflow-y-auto rounded-md border bg-popover shadow-md"
    >
      {children}
    </div>,
    document.body,
  )
}
