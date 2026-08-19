import { useCallback, useEffect, useRef, type DependencyList } from 'react'
import * as echarts from 'echarts'
import type { EChartsOption } from 'echarts'

/**
 * ECharts 生命周期 hook：init → resize 监听 → dispose。
 *
 * 用 callback ref 而非空依赖 effect：图表容器可能是条件渲染的
 * （如数据加载完成后才出现），空依赖 effect 只跑一次，挂载时容器
 * 还不存在会跳过 init，导致图表永远空白。callback ref 在节点真正
 * 挂载/卸载时调用，容器出现再 init 也能工作。
 */
export function useEChart(option: EChartsOption | null, deps: DependencyList) {
  const chartRef = useRef<echarts.ECharts | null>(null)
  // 最新 option 存 ref：callback ref 在渲染周期外触发，直接读闭包会拿到旧值
  const optionRef = useRef<EChartsOption | null>(option)
  optionRef.current = option

  const domRef = useCallback((node: HTMLDivElement | null) => {
    chartRef.current?.dispose()
    chartRef.current = null
    if (node) {
      chartRef.current = echarts.init(node)
      if (optionRef.current) chartRef.current.setOption(optionRef.current, true)
    }
  }, [])

  // 窗口 resize 跟随；卸载时 dispose（StrictMode 双挂载安全：先 dispose 再 init）
  useEffect(() => {
    const onResize = () => chartRef.current?.resize()
    window.addEventListener('resize', onResize)
    return () => {
      window.removeEventListener('resize', onResize)
      chartRef.current?.dispose()
      chartRef.current = null
    }
  }, [])

  // 更新：notMerge 全量替换，切换 adjust/范围时无残留
  useEffect(() => {
    if (option && chartRef.current) chartRef.current.setOption(option, true)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps)

  return domRef
}
