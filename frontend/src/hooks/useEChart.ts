import { useEffect, useRef, type DependencyList, type RefObject } from 'react'
import * as echarts from 'echarts'
import type { EChartsOption } from 'echarts'

/**
 * ECharts 生命周期 hook：init → resize 监听 → dispose 兜底（StrictMode 双挂载安全），
 * option 通过第二个 effect 以 notMerge 全量替换，由 deps 驱动更新。
 * 容器高度由父级决定（div style height 非零），option 为 null 时不初始化。
 */
export function useEChart(option: EChartsOption | null, deps: DependencyList): RefObject<HTMLDivElement> {
  const ref = useRef<HTMLDivElement>(null)

  // 挂载：init + resize；卸载：dispose
  useEffect(() => {
    if (!ref.current) return
    const chart = echarts.init(ref.current)
    const onResize = () => chart.resize()
    window.addEventListener('resize', onResize)
    return () => {
      window.removeEventListener('resize', onResize)
      chart.dispose()
    }
  }, [])

  // 更新：notMerge 全量替换，切换 adjust/范围时无残留
  useEffect(() => {
    if (!ref.current || !option) return
    echarts.getInstanceByDom(ref.current)?.setOption(option, true)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps)

  return ref
}
