import { useEffect, useRef } from 'react'
import { BarChart, LineChart, PieChart } from 'echarts/charts'
import {
  GridComponent,
  LegendComponent,
  TooltipComponent,
  TitleComponent,
  GraphicComponent,
  AriaComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import {
  init,
  use as registerECharts,
  type ECharts,
  type ECElementEvent,
  type EChartsCoreOption,
  type SetOptionOpts,
} from 'echarts/core'

registerECharts([
  BarChart,
  LineChart,
  PieChart,
  GridComponent,
  LegendComponent,
  TooltipComponent,
  TitleComponent,
  GraphicComponent,
  AriaComponent,
  CanvasRenderer,
])

type EChartProps = {
  option: EChartsCoreOption
  height: number | string
  width?: number | string
  className?: string
  settings?: SetOptionOpts
  onClick?: (params: ECElementEvent) => void
  ariaLabel?: string
}

export function EChart({ option, height, width = '100%', className, settings, onClick, ariaLabel = 'Визуализация данных' }: EChartProps) {
  const containerRef = useRef<HTMLDivElement | null>(null)
  const chartRef = useRef<ECharts | null>(null)

  useEffect(() => {
    if (!containerRef.current) return

    const chart = init(containerRef.current, undefined, {
      renderer: 'canvas',
    })
    chartRef.current = chart

    const observer = new ResizeObserver(() => {
      chart.resize()
    })
    observer.observe(containerRef.current)

    return () => {
      observer.disconnect()
      chart.dispose()
      chartRef.current = null
    }
  }, [])

  useEffect(() => {
    if (!chartRef.current) return
    chartRef.current.setOption({ aria: { enabled: true }, ...option }, settings ?? { notMerge: true })
  }, [option, settings])

  useEffect(() => {
    if (!chartRef.current || !onClick) return

    const chart = chartRef.current
    chart.on('click', onClick)

    return () => {
      chart.off('click', onClick)
    }
  }, [onClick])

  return (
    <div
      ref={containerRef}
      className={className}
      style={{ width, height }}
      role="img"
      aria-label={ariaLabel}
    />
  )
}
