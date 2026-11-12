import * as React from "react"

// 判定"主输入是不是触屏"，用于输入法行为（Enter 换行 vs 发送）。
// 刻意不看视口宽度：桌面窗口缩小 / Edge 非全屏时，鼠标仍然是鼠标。
export function useIsTouchPrimary() {
  const [isTouch, setIsTouch] = React.useState(false)

  React.useEffect(() => {
    const mql = window.matchMedia("(pointer: coarse)")
    const onChange = () => setIsTouch(mql.matches)
    mql.addEventListener("change", onChange)
    setIsTouch(mql.matches)
    return () => mql.removeEventListener("change", onChange)
  }, [])

  return isTouch
}
