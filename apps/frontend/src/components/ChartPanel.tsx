import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Button, Card, Empty, Select, Space, Tag, Tooltip } from 'antd'
import {
  CandlestickSeries,
  ColorType,
  LineSeries,
  createChart,
  type BusinessDay,
  type IChartApi,
  type ISeriesApi,
  type Time,
} from 'lightweight-charts'
import {
  BorderOutlined,
  CompressOutlined,
  ExpandOutlined,
  MinusOutlined,
  PlusOutlined,
} from '@ant-design/icons'
import dayjs, { type Dayjs } from 'dayjs'
import isoWeek from 'dayjs/plugin/isoWeek'
import type { OHLCRow } from '../types/api'

dayjs.extend(isoWeek)

const riseColor = '#d92d20'
const fallColor = '#16a34a'
const crosshairColor = 'rgba(217, 45, 32, 0.5)'

type Timeframe = 'day' | 'week' | 'month'
type IndicatorKey = 'volume' | 'ma' | 'rsi' | 'boll' | 'kdj' | 'macd'

interface DisplayRow extends OHLCRow {
  startDate: string
  endDate: string
}

interface IndicatorPoint {
  date: string
  value: number | null
}

interface MACDPoint {
  date: string
  macd: number | null
  signal: number | null
  histogram: number | null
}

interface KDJPoint {
  date: string
  k: number | null
  d: number | null
  j: number | null
}

interface BOLLPoint {
  date: string
  middle: number | null
  upper: number | null
  lower: number | null
}

const timeframeLabels: Record<Timeframe, string> = {
  day: '日线',
  week: '周线',
  month: '月线',
}

const timeframeDescriptions: Record<Timeframe, string> = {
  day: '日线，每根代表 1 个交易日',
  week: '周线，按交易周聚合',
  month: '月线，按自然月聚合',
}

const defaultWindowMap: Record<Timeframe, number> = {
  day: 30,
  week: 30,
  month: 12,
}

function clampRange(length: number, start: number, end: number) {
  if (!length) {
    return { from: 0, to: 0 }
  }
  const maxIdx = length - 1
  const from = Math.max(0, Math.min(start, maxIdx))
  const to = Math.max(0, Math.min(end, maxIdx))
  return from <= to ? { from, to } : { from: to, to: from }
}

function formatNumber(value?: number) {
  if (value === undefined) {
    return '--'
  }
  return new Intl.NumberFormat('zh-CN', {
    maximumFractionDigits: 2,
  }).format(value)
}

function formatVolume(value?: number) {
  if (value === undefined) {
    return '--'
  }
  if (value >= 1_000_000_000) {
    return `${(value / 1_000_000_000).toFixed(2)}B`
  }
  if (value >= 1_000_000) {
    return `${(value / 1_000_000).toFixed(2)}M`
  }
  if (value >= 1_000) {
    return `${(value / 1_000).toFixed(2)}K`
  }
  return value.toFixed(0)
}

function formatSigned(value?: number | null) {
  if (value === undefined || value === null || Number.isNaN(value)) {
    return '--'
  }
  const sign = value > 0 ? '+' : ''
  return `${sign}${value.toFixed(2)}`
}

function buildBucketKey(date: Dayjs, timeframe: Timeframe) {
  if (timeframe === 'week') {
    return `${date.isoWeekYear()}-W${date.isoWeek()}`
  }
  if (timeframe === 'month') {
    return date.format('YYYY-MM')
  }
  return date.format('YYYY-MM-DD')
}

function toChartTime(value: string): BusinessDay {
  const [year, month, day] = value.split('-').map((item) => Number(item))
  return {
    year,
    month,
    day,
  }
}

function aggregateRows(rows: OHLCRow[], timeframe: Timeframe): DisplayRow[] {
  if (timeframe === 'day') {
    return rows.map((row) => ({
      ...row,
      startDate: row.date,
      endDate: row.date,
    }))
  }

  const buckets = new Map<string, DisplayRow>()

  for (const row of rows) {
    const date = dayjs(row.date)
    const key = buildBucketKey(date, timeframe)
    const existing = buckets.get(key)

    if (!existing) {
      buckets.set(key, {
        ...row,
        startDate: row.date,
        endDate: row.date,
      })
      continue
    }

    existing.high = Math.max(existing.high, row.high)
    existing.low = Math.min(existing.low, row.low)
    existing.close = row.close
    existing.volume += row.volume
    existing.amount = (existing.amount ?? 0) + (row.amount ?? 0)
    existing.change_rate = row.change_rate
    existing.date = row.date
    existing.endDate = row.date
  }

  return Array.from(buckets.values())
}

function computeEMA(values: number[], period: number): Array<number | null> {
  const result = new Array<number | null>(values.length).fill(null)
  if (values.length < period) {
    return result
  }

  const multiplier = 2 / (period + 1)
  let previous = values.slice(0, period).reduce((sum, value) => sum + value, 0) / period
  result[period - 1] = previous

  for (let i = period; i < values.length; i += 1) {
    previous = (values[i] - previous) * multiplier + previous
    result[i] = previous
  }

  return result
}

function computeRSI(rows: DisplayRow[], period = 14): IndicatorPoint[] {
  const closes = rows.map((row) => row.close)
  const result = rows.map((row) => ({ date: row.date, value: null as number | null }))

  if (closes.length <= period) {
    return result
  }

  let gains = 0
  let losses = 0
  for (let i = 1; i <= period; i += 1) {
    const delta = closes[i] - closes[i-1]
    if (delta >= 0) gains += delta
    if (delta < 0) losses += Math.abs(delta)
  }

  let averageGain = gains / period
  let averageLoss = losses / period
  result[period].value = averageLoss == 0 ? 100 : 100 - (100 / (1 + averageGain / averageLoss))

  for (let i = period + 1; i < closes.length; i += 1) {
    const delta = closes[i] - closes[i-1]
    const gain = delta > 0 ? delta : 0
    const loss = delta < 0 ? Math.abs(delta) : 0
    averageGain = ((averageGain * (period - 1)) + gain) / period
    averageLoss = ((averageLoss * (period - 1)) + loss) / period
    result[i].value = averageLoss == 0 ? 100 : 100 - (100 / (1 + averageGain / averageLoss))
  }

  return result
}

function computeMACD(rows: DisplayRow[]): MACDPoint[] {
  const closes = rows.map((row) => row.close)
  const ema12 = computeEMA(closes, 12)
  const ema26 = computeEMA(closes, 26)
  const macdValues = closes.map((_, index) =>
    ema12[index] !== null && ema26[index] !== null ? ema12[index]! - ema26[index]! : null,
  )

  const signalInput = macdValues.map((value) => value ?? 0)
  const signalRaw = computeEMA(signalInput, 9)

  return rows.map((row, index) => {
    const macd = macdValues[index]
    const signal = macd === null || signalRaw[index] === null ? null : signalRaw[index]
    return {
      date: row.date,
      macd,
      signal,
      histogram: macd !== null && signal !== null ? macd - signal : null,
    }
  })
}

function computeMA(rows: DisplayRow[], period: number): IndicatorPoint[] {
  return rows.map((row, index) => {
    if (index + 1 < period) {
      return { date: row.date, value: null }
    }
    const slice = rows.slice(index - period + 1, index + 1)
    const total = slice.reduce((sum, item) => sum + item.close, 0)
    return { date: row.date, value: total / period }
  })
}

function computeKDJ(rows: DisplayRow[], period = 9): KDJPoint[] {
  let previousK = 50
  let previousD = 50

  return rows.map((row, index) => {
    if (index + 1 < period) {
      return { date: row.date, k: null, d: null, j: null }
    }

    const window = rows.slice(index - period + 1, index + 1)
    const low = Math.min(...window.map((item) => item.low))
    const high = Math.max(...window.map((item) => item.high))
    const rsv = high === low ? 50 : ((row.close - low) / (high - low)) * 100
    const k = (2 / 3) * previousK + (1 / 3) * rsv
    const d = (2 / 3) * previousD + (1 / 3) * k
    const j = 3 * k - 2 * d

    previousK = k
    previousD = d

    return { date: row.date, k, d, j }
  })
}

function computeBOLL(rows: DisplayRow[], period = 20, multiplier = 2): BOLLPoint[] {
  return rows.map((row, index) => {
    if (index + 1 < period) {
      return { date: row.date, middle: null, upper: null, lower: null }
    }

    const window = rows.slice(index - period + 1, index + 1)
    const middle = window.reduce((sum, item) => sum + item.close, 0) / period
    const variance =
      window.reduce((sum, item) => sum + (item.close - middle) ** 2, 0) / period
    const deviation = Math.sqrt(variance)

    return {
      date: row.date,
      middle,
      upper: middle + deviation * multiplier,
      lower: middle - deviation * multiplier,
    }
  })
}

function buildPolyline(points: Array<number | null>, height: number, width: number) {
  const valid = points
    .map((value, index) => ({ value, index }))
    .filter((item) => item.value !== null) as Array<{ value: number; index: number }>

  if (valid.length < 2) {
    return ''
  }

  const min = Math.min(...valid.map((item) => item.value))
  const max = Math.max(...valid.map((item) => item.value))
  const span = max - min || 1

  return valid
    .map((item) => {
      const x = (item.index / Math.max(points.length - 1, 1)) * width
      const y = height - ((item.value - min) / span) * height
      return `${x},${y}`
    })
    .join(' ')
}

function buildPricePolyline(
  points: Array<number | null>,
  min: number,
  max: number,
  height: number,
  width: number,
) {
  const valid = points
    .map((value, index) => ({ value, index }))
    .filter((item) => item.value !== null) as Array<{ value: number; index: number }>

  if (valid.length < 2) {
    return ''
  }

  const span = max - min || 1

  return valid
    .map((item) => {
      const x = (item.index / Math.max(points.length - 1, 1)) * width
      const y = height - ((item.value - min) / span) * height
      return `${x},${y}`
    })
    .join(' ')
}

function buildPricePolylineInFrame(
  points: Array<number | null>,
  min: number,
  max: number,
  frame: { left: number; top: number; width: number; height: number },
) {
  const valid = points
    .map((value, index) => ({ value, index }))
    .filter((item) => item.value !== null) as Array<{ value: number; index: number }>

  if (valid.length < 2) {
    return ''
  }

  const span = max - min || 1

  return valid
    .map((item) => {
      const x = frame.left + (item.index / Math.max(points.length - 1, 1)) * frame.width
      const y = frame.top + frame.height - (((item.value - min) / span) * frame.height)
      return `${x},${y}`
    })
    .join(' ')
}

function mapPriceToFrameY(
  price: number,
  min: number,
  max: number,
  frame: { top: number; height: number },
) {
  const span = max - min || 1
  return frame.top + frame.height - (((price - min) / span) * frame.height)
}

interface ChartPanelProps {
  rows: OHLCRow[]
  range: [Dayjs | null, Dayjs | null] | null
  onRangeChange: (range: [Dayjs | null, Dayjs | null] | null) => void
  isLoading?: boolean
}

export function ChartPanel({
  rows,
  range,
  onRangeChange,
  isLoading = false,
}: ChartPanelProps) {
  const containerRef = useRef<HTMLDivElement | null>(null)
  const overlayRef = useRef<HTMLDivElement | null>(null)
  const chartRef = useRef<IChartApi | null>(null)
  const seriesRef = useRef<ISeriesApi<'Candlestick', Time> | null>(null)
  const ma5OverlayRef = useRef<ISeriesApi<'Line', Time> | null>(null)
  const ma10OverlayRef = useRef<ISeriesApi<'Line', Time> | null>(null)
  const ma20OverlayRef = useRef<ISeriesApi<'Line', Time> | null>(null)
  const displayRowsRef = useRef<DisplayRow[]>([])
  const visibleRangeRef = useRef<{ from: number; to: number } | null>(null)
  const timeframeRef = useRef<Timeframe>('day')
  const previousTimeframeRef = useRef<Timeframe>('day')
  const previousDataLengthRef = useRef(0)
  const panOriginRef = useRef<{ pointerX: number; range: { from: number; to: number } } | null>(null)
  const [dragStartX, setDragStartX] = useState<number | null>(null)
  const [dragCurrentX, setDragCurrentX] = useState<number | null>(null)
  const [hoveredIndex, setHoveredIndex] = useState<number | null>(null)
  const [timeframe, setTimeframe] = useState<Timeframe>('day')
  const [visibleWindow, setVisibleWindow] = useState<{ from: number; to: number } | null>(null)
  const [interactionMode, setInteractionMode] = useState<'browse' | 'select'>('browse')
  const [overlayVisibility, setOverlayVisibility] = useState({
    ma5: true,
    ma10: true,
    ma20: true,
  })
  const [selectedIndicators, setSelectedIndicators] = useState<IndicatorKey[]>(['volume'])

  const displayRows = useMemo(() => aggregateRows(rows, timeframe), [rows, timeframe])
  const rsiSeries = useMemo(() => computeRSI(displayRows), [displayRows])
  const macdSeries = useMemo(() => computeMACD(displayRows), [displayRows])
  const ma5Series = useMemo(() => computeMA(displayRows, 5), [displayRows])
  const ma10Series = useMemo(() => computeMA(displayRows, 10), [displayRows])
  const ma20Series = useMemo(() => computeMA(displayRows, 20), [displayRows])
  const kdjSeries = useMemo(() => computeKDJ(displayRows), [displayRows])
  const bollSeries = useMemo(() => computeBOLL(displayRows), [displayRows])

  const computeSpanRange = useCallback((frame: Timeframe, length: number) => {
    const span = defaultWindowMap[frame]
    const to = length - 1
    const from = Math.max(0, to - span + 1)
    return { from, to }
  }, [])

  const visibleSlice = useMemo(() => {
    if (!displayRows.length) {
      return { from: 0, to: 0 }
    }
    if (!visibleWindow) {
      return computeSpanRange(timeframe, displayRows.length)
    }
    return clampRange(displayRows.length, visibleWindow.from, visibleWindow.to)
  }, [computeSpanRange, displayRows.length, timeframe, visibleWindow])


  const visibleDisplayRows = useMemo(
    () => displayRows.slice(visibleSlice.from, visibleSlice.to + 1),
    [displayRows, visibleSlice.from, visibleSlice.to],
  )

  const visibleRSI = useMemo(
    () => rsiSeries.slice(visibleSlice.from, visibleSlice.to + 1),
    [rsiSeries, visibleSlice.from, visibleSlice.to],
  )

  const visibleMACD = useMemo(
    () => macdSeries.slice(visibleSlice.from, visibleSlice.to + 1),
    [macdSeries, visibleSlice.from, visibleSlice.to],
  )

  const visibleMA5 = useMemo(
    () => ma5Series.slice(visibleSlice.from, visibleSlice.to + 1),
    [ma5Series, visibleSlice.from, visibleSlice.to],
  )

  const visibleMA10 = useMemo(
    () => ma10Series.slice(visibleSlice.from, visibleSlice.to + 1),
    [ma10Series, visibleSlice.from, visibleSlice.to],
  )

  const visibleMA20 = useMemo(
    () => ma20Series.slice(visibleSlice.from, visibleSlice.to + 1),
    [ma20Series, visibleSlice.from, visibleSlice.to],
  )

  const visibleKDJ = useMemo(
    () => kdjSeries.slice(visibleSlice.from, visibleSlice.to + 1),
    [kdjSeries, visibleSlice.from, visibleSlice.to],
  )

  const visibleBOLL = useMemo(
    () => bollSeries.slice(visibleSlice.from, visibleSlice.to + 1),
    [bollSeries, visibleSlice.from, visibleSlice.to],
  )

  const visibleVolumeMax = useMemo(
    () => Math.max(...visibleDisplayRows.map((item) => item.volume), 0),
    [visibleDisplayRows],
  )
  const priceDomain = useMemo(() => {
    if (!visibleDisplayRows.length) {
      return { min: 0, max: 1 }
    }
    const high = Math.max(...visibleDisplayRows.map((item) => item.high))
    const low = Math.min(...visibleDisplayRows.map((item) => item.low))
    const padding = (high - low || high || 1) * 0.06
    return {
      min: low - padding,
      max: high + padding,
    }
  }, [visibleDisplayRows])

  const statsRows = useMemo(() => {
    if (!range?.[0] || !range?.[1]) {
      return rows
    }
    const start = dayjs(range[0])
    const end = dayjs(range[1])
    return rows.filter((row) => {
      const date = dayjs(row.date)
      return !date.isBefore(start) && !date.isAfter(end)
    })
  }, [rows, range])

  const displayStatsRows = useMemo(() => {
    if (!range?.[0] || !range?.[1]) {
      return displayRows
    }
    const start = dayjs(range[0])
    const end = dayjs(range[1])
    return displayRows.filter((row) => {
      const rowStart = dayjs(row.startDate)
      const rowEnd = dayjs(row.endDate)
      return !rowEnd.isBefore(start) && !rowStart.isAfter(end)
    })
  }, [displayRows, range])

  const stats = useMemo(() => {
    if (!statsRows.length) {
      return null
    }
    const first = statsRows[0]
    const last = statsRows[statsRows.length - 1]
    const high = Math.max(...statsRows.map((row) => row.high))
    const low = Math.min(...statsRows.map((row) => row.low))
    const totalVolume = statsRows.reduce((sum, row) => sum + row.volume, 0)
    const pct = first.close === 0 ? 0 : ((last.close - first.close) / first.close) * 100
    return {
      lastClose: last.close,
      high,
      low,
      pct,
      candleCount: displayStatsRows.length,
      volume: totalVolume,
    }
  }, [displayStatsRows.length, statsRows])

  const applyVisibleRange = useCallback((rangeValue: { from: number; to: number }) => {
    const currentRows = displayRowsRef.current
    if (!currentRows.length) {
      return
    }
    const next = clampRange(currentRows.length, rangeValue.from, rangeValue.to)
    visibleRangeRef.current = next
    setVisibleWindow(next)
    const chart = chartRef.current
    if (chart) {
      chart.timeScale().setVisibleLogicalRange({
        from: next.from,
        to: next.to,
      })
    }
  }, [])

  const zoomRange = useCallback(
    (scale: number) => {
      const currentRows = displayRowsRef.current
      if (!currentRows.length) {
        return
      }
      const current =
        chartRef.current?.timeScale().getVisibleLogicalRange() ??
        visibleRangeRef.current ??
        computeSpanRange(timeframeRef.current, currentRows.length)
      const center = (current.from + current.to) / 2
      const width = Math.max(6, (current.to - current.from + 1) * scale)
      applyVisibleRange({
        from: Math.round(center - width / 2),
        to: Math.round(center + width / 2),
      })
    },
    [applyVisibleRange, computeSpanRange],
  )

  useEffect(() => {
    if (!containerRef.current || chartRef.current) {
      return
    }

    const container = containerRef.current
    const chart = createChart(container, {
      width: Math.max(container.clientWidth, 720),
      height: 420,
      layout: {
        background: { type: ColorType.Solid, color: '#fffdf8' },
        textColor: '#334155',
      },
      grid: {
        vertLines: { color: '#e7e5e4' },
        horzLines: { color: '#e7e5e4' },
      },
      rightPriceScale: {
        borderColor: '#d6d3d1',
      },
      timeScale: {
        borderColor: '#d6d3d1',
        barSpacing: 9,
        minBarSpacing: 5,
        rightOffset: 0,
        fixLeftEdge: true,
        fixRightEdge: true,
      },
      crosshair: {
        vertLine: { color: riseColor, width: 1 },
        horzLine: { color: riseColor, width: 1 },
      },
      localization: {
        locale: 'zh-CN',
        dateFormat: 'yyyy-MM-dd',
      },
    })

    const series = chart.addSeries(CandlestickSeries, {
      upColor: riseColor,
      borderUpColor: riseColor,
      wickUpColor: riseColor,
      downColor: fallColor,
      borderDownColor: fallColor,
      wickDownColor: fallColor,
      priceLineVisible: false,
    })

    const ma5Overlay = chart.addSeries(LineSeries, {
      color: '#0f766e',
      lineWidth: 2,
      priceLineVisible: false,
      lastValueVisible: false,
      crosshairMarkerVisible: false,
    })
    const ma10Overlay = chart.addSeries(LineSeries, {
      color: '#1d4ed8',
      lineWidth: 2,
      priceLineVisible: false,
      lastValueVisible: false,
      crosshairMarkerVisible: false,
    })
    const ma20Overlay = chart.addSeries(LineSeries, {
      color: '#f59e0b',
      lineWidth: 2,
      priceLineVisible: false,
      lastValueVisible: false,
      crosshairMarkerVisible: false,
    })

    const resizeObserver = new ResizeObserver((entries) => {
      const entry = entries[0]
      if (!entry) {
        return
      }
      chart.resize(Math.max(entry.contentRect.width, 720), 420)
    })
    resizeObserver.observe(container)

    chartRef.current = chart
    seriesRef.current = series
    ma5OverlayRef.current = ma5Overlay
    ma10OverlayRef.current = ma10Overlay
    ma20OverlayRef.current = ma20Overlay

    return () => {
      resizeObserver.disconnect()
      seriesRef.current = null
      ma5OverlayRef.current = null
      ma10OverlayRef.current = null
      ma20OverlayRef.current = null
      chartRef.current = null
      chart.remove()
    }
  }, [])

  useEffect(() => {
    const previousTimeframe = previousTimeframeRef.current
    const previousLength = previousDataLengthRef.current
    timeframeRef.current = timeframe
    displayRowsRef.current = displayRows

    const chart = chartRef.current
    const series = seriesRef.current
    const ma5Overlay = ma5OverlayRef.current
    const ma10Overlay = ma10OverlayRef.current
    const ma20Overlay = ma20OverlayRef.current
    if (!chart || !series || !ma5Overlay || !ma10Overlay || !ma20Overlay) {
      return
    }

    if (!displayRows.length) {
      series.setData([])
      ma5Overlay.setData([])
      ma10Overlay.setData([])
      ma20Overlay.setData([])
      visibleRangeRef.current = null
      return
    }

    series.setData(
      displayRows.map((row) => ({
        time: toChartTime(row.date) as Time,
        open: row.open,
        high: row.high,
        low: row.low,
        close: row.close,
      })),
    )
    ma5Overlay.setData(
      overlayVisibility.ma5
        ? ma5Series
            .filter((row) => row.value !== null)
            .map((row) => ({ time: toChartTime(row.date) as Time, value: row.value! }))
        : [],
    )
    ma10Overlay.setData(
      overlayVisibility.ma10
        ? ma10Series
            .filter((row) => row.value !== null)
            .map((row) => ({ time: toChartTime(row.date) as Time, value: row.value! }))
        : [],
    )
    ma20Overlay.setData(
      overlayVisibility.ma20
        ? ma20Series
            .filter((row) => row.value !== null)
            .map((row) => ({ time: toChartTime(row.date) as Time, value: row.value! }))
        : [],
    )

    chart.applyOptions({
      timeScale: {
        timeVisible: timeframe !== 'month',
        secondsVisible: false,
      },
    })

    const timeframeChanged = previousTimeframe !== timeframe
    const dataShapeChanged = previousLength !== displayRows.length
    const currentRange = visibleRangeRef.current
    const rangeInvalid = currentRange !== null && (
      currentRange.from >= displayRows.length ||
      currentRange.to >= displayRows.length
    )

    const nextRange =
      currentRange && !timeframeChanged && !dataShapeChanged && !rangeInvalid
        ? currentRange
        : computeSpanRange(timeframe, displayRows.length)

    requestAnimationFrame(() => {
      chart.resize(Math.max(containerRef.current?.clientWidth ?? 720, 720), 420)
      chart.timeScale().fitContent()
      requestAnimationFrame(() => {
        applyVisibleRange(nextRange)
      })
    })
    previousTimeframeRef.current = timeframe
    previousDataLengthRef.current = displayRows.length
    setHoveredIndex(null)
  }, [applyVisibleRange, computeSpanRange, displayRows, ma10Series, ma20Series, ma5Series, overlayVisibility, timeframe])

  useEffect(() => {
    const node = overlayRef.current
    if (!node) {
      return
    }

    const handleWheel = (event: WheelEvent) => {
      event.preventDefault()
      zoomRange(event.deltaY > 0 ? 1.18 : 0.84)
    }

    node.addEventListener('wheel', handleWheel, { passive: false })
    return () => node.removeEventListener('wheel', handleWheel)
  }, [zoomRange])

  const handleOverlayWheel = useCallback((event: React.WheelEvent<HTMLDivElement>) => {
    event.preventDefault()
    event.stopPropagation()
    zoomRange(event.deltaY > 0 ? 1.18 : 0.84)
  }, [zoomRange])

  const clampDragIndex = (clientX: number) => {
    const currentRows = displayRowsRef.current
    if (!overlayRef.current || !currentRows.length) {
      return 0
    }
    const rect = overlayRef.current.getBoundingClientRect()
    const x = Math.min(Math.max(clientX - rect.left, 0), rect.width)
    const visible =
      visibleRangeRef.current ?? computeSpanRange(timeframeRef.current, currentRows.length)
    const span = Math.max(visible.to - visible.from, 1)
    const ratio = rect.width <= 0 ? 0 : x / rect.width
    const index = Math.round(visible.from + span * ratio)
    return clampRange(currentRows.length, index, index).from
  }

  const handlePointerDown = (event: React.PointerEvent<HTMLDivElement>) => {
    setHoveredIndex(clampDragIndex(event.clientX))
    if (event.button === 2 && interactionMode === 'browse') {
      const currentRows = displayRowsRef.current
      if (!currentRows.length) {
        return
      }
      const currentRange = visibleRangeRef.current ?? computeSpanRange(timeframeRef.current, currentRows.length)
      panOriginRef.current = {
        pointerX: event.clientX,
        range: currentRange,
      }
      overlayRef.current?.setPointerCapture(event.pointerId)
      return
    }
    if (interactionMode !== 'select') {
      return
    }
    overlayRef.current?.setPointerCapture(event.pointerId)
    setDragStartX(event.clientX)
    setDragCurrentX(event.clientX)
  }

  const handlePointerMove = (event: React.PointerEvent<HTMLDivElement>) => {
    setHoveredIndex(clampDragIndex(event.clientX))
    if (panOriginRef.current && overlayRef.current) {
      const rect = overlayRef.current.getBoundingClientRect()
      const span = Math.max(panOriginRef.current.range.to - panOriginRef.current.range.from, 1)
      const deltaRatio = rect.width <= 0 ? 0 : (event.clientX - panOriginRef.current.pointerX) / rect.width
      const deltaBars = Math.round(deltaRatio * span)
      applyVisibleRange({
        from: panOriginRef.current.range.from - deltaBars,
        to: panOriginRef.current.range.to - deltaBars,
      })
      return
    }
    if (interactionMode !== 'select' || dragStartX === null) {
      return
    }
    setDragCurrentX(event.clientX)
  }

  const handlePointerUp = (event: React.PointerEvent<HTMLDivElement>) => {
    if (panOriginRef.current) {
      overlayRef.current?.releasePointerCapture(event.pointerId)
      panOriginRef.current = null
      return
    }
    if (interactionMode !== 'select') {
      return
    }
    if (dragStartX === null) {
      return
    }

    overlayRef.current?.releasePointerCapture(event.pointerId)
    const currentRows = displayRowsRef.current
    const startIndex = clampDragIndex(dragStartX)
    const endIndex = clampDragIndex(event.clientX)
    const { from, to } = clampRange(currentRows.length, startIndex, endIndex)
    if (from !== to) {
      onRangeChange([dayjs(currentRows[from].startDate), dayjs(currentRows[to].endDate)])
    }
    setInteractionMode('browse')
    setDragStartX(null)
    setDragCurrentX(null)
  }

  const handlePointerLeave = () => {
    setHoveredIndex(null)
    setDragStartX(null)
    setDragCurrentX(null)
    panOriginRef.current = null
  }

  const handleDoubleClick = () => {
    const currentRows = displayRowsRef.current
    if (!currentRows.length) {
      return
    }
    applyVisibleRange(computeSpanRange(timeframe, currentRows.length))
  }

  const selectionStyle = useMemo(() => {
    if (interactionMode !== 'select' || dragStartX === null || dragCurrentX === null || !overlayRef.current) {
      return null
    }
    const rect = overlayRef.current.getBoundingClientRect()
    const left = Math.max(Math.min(dragStartX, dragCurrentX) - rect.left, 0)
    const right = Math.min(Math.max(dragStartX, dragCurrentX) - rect.left, rect.width)
    return { left, width: Math.max(right - left, 2) }
  }, [dragCurrentX, dragStartX, interactionMode])

  const selectionLabel = useMemo(() => {
    if (!range?.[0] || !range?.[1]) {
      return '拖拽图表框选区间'
    }
    return `${range[0].format('MM-DD')} -> ${range[1].format('MM-DD')}`
  }, [range])

  const latestRSI = visibleRSI.findLast((item) => item.value !== null)?.value ?? null
  const latestMACD = visibleMACD.findLast((item) => item.macd !== null) ?? null
  const latestMA5 = visibleMA5.findLast((item) => item.value !== null)?.value ?? null
  const latestMA10 = visibleMA10.findLast((item) => item.value !== null)?.value ?? null
  const latestMA20 = visibleMA20.findLast((item) => item.value !== null)?.value ?? null
  const latestKDJ = visibleKDJ.findLast((item) => item.k !== null) ?? null
  const latestBOLL = visibleBOLL.findLast((item) => item.middle !== null) ?? null
  const activeIndex =
    hoveredIndex === null
      ? visibleDisplayRows.length - 1
      : Math.max(0, Math.min(hoveredIndex - visibleSlice.from, visibleDisplayRows.length - 1))
  const activeRow = visibleDisplayRows[activeIndex] ?? null
  const rsiPolyline = buildPolyline(
    visibleRSI.map((item) => item.value),
    88,
    520,
  )
  const macdLine = buildPolyline(
    visibleMACD.map((item) => item.macd),
    88,
    520,
  )
  const signalLine = buildPolyline(
    visibleMACD.map((item) => item.signal),
    88,
    520,
  )
  const priceLine = buildPolyline(
    visibleDisplayRows.map((item) => item.close),
    88,
    520,
  )
  const ma5Line = buildPolyline(
    visibleMA5.map((item) => item.value),
    88,
    520,
  )
  const ma10Line = buildPolyline(
    visibleMA10.map((item) => item.value),
    88,
    520,
  )
  const ma20Line = buildPolyline(
    visibleMA20.map((item) => item.value),
    88,
    520,
  )
  const kLine = buildPolyline(
    visibleKDJ.map((item) => item.k),
    88,
    520,
  )
  const dLine = buildPolyline(
    visibleKDJ.map((item) => item.d),
    88,
    520,
  )
  const jLine = buildPolyline(
    visibleKDJ.map((item) => item.j),
    88,
    520,
  )
  const bollMiddleLine = buildPolyline(
    visibleBOLL.map((item) => item.middle),
    88,
    520,
  )
  const bollUpperLine = buildPolyline(
    visibleBOLL.map((item) => item.upper),
    88,
    520,
  )
  const bollLowerLine = buildPolyline(
    visibleBOLL.map((item) => item.lower),
    88,
    520,
  )
  const mainChartWidth = 960
  const mainChartHeight = 420
  const chartFrame = {
    left: 20,
    right: 24,
    top: 18,
    bottom: 46,
  }
  const plotWidth = mainChartWidth - chartFrame.left - chartFrame.right
  const plotHeight = mainChartHeight - chartFrame.top - chartFrame.bottom
  const candleSlot = plotWidth / Math.max(visibleDisplayRows.length, 1)
  const candleBodyWidth = Math.min(Math.max(candleSlot * 0.56, 4), 16)
  const ma5LineMain = buildPricePolyline(
    visibleMA5.map((item) => item.value),
    priceDomain.min,
    priceDomain.max,
    mainChartHeight,
    mainChartWidth,
  )
  const ma10LineMain = buildPricePolyline(
    visibleMA10.map((item) => item.value),
    priceDomain.min,
    priceDomain.max,
    mainChartHeight,
    mainChartWidth,
  )
  const ma20LineMain = buildPricePolyline(
    visibleMA20.map((item) => item.value),
    priceDomain.min,
    priceDomain.max,
    mainChartHeight,
    mainChartWidth,
  )
  const priceLineFramed = buildPricePolylineInFrame(
    visibleDisplayRows.map((item) => item.close),
    priceDomain.min,
    priceDomain.max,
    { left: chartFrame.left, top: chartFrame.top, width: plotWidth, height: plotHeight },
  )
  const ma5LineFramed = buildPricePolylineInFrame(
    visibleMA5.map((item) => item.value),
    priceDomain.min,
    priceDomain.max,
    { left: chartFrame.left, top: chartFrame.top, width: plotWidth, height: plotHeight },
  )
  const ma10LineFramed = buildPricePolylineInFrame(
    visibleMA10.map((item) => item.value),
    priceDomain.min,
    priceDomain.max,
    { left: chartFrame.left, top: chartFrame.top, width: plotWidth, height: plotHeight },
  )
  const ma20LineFramed = buildPricePolylineInFrame(
    visibleMA20.map((item) => item.value),
    priceDomain.min,
    priceDomain.max,
    { left: chartFrame.left, top: chartFrame.top, width: plotWidth, height: plotHeight },
  )
  const bollUpperLineFramed = buildPricePolylineInFrame(
    visibleBOLL.map((item) => item.upper),
    priceDomain.min,
    priceDomain.max,
    { left: chartFrame.left, top: chartFrame.top, width: plotWidth, height: plotHeight },
  )
  const bollMiddleLineFramed = buildPricePolylineInFrame(
    visibleBOLL.map((item) => item.middle),
    priceDomain.min,
    priceDomain.max,
    { left: chartFrame.left, top: chartFrame.top, width: plotWidth, height: plotHeight },
  )
  const bollLowerLineFramed = buildPricePolylineInFrame(
    visibleBOLL.map((item) => item.lower),
    priceDomain.min,
    priceDomain.max,
    { left: chartFrame.left, top: chartFrame.top, width: plotWidth, height: plotHeight },
  )
  const getPlotX = (index: number) =>
    chartFrame.left + (index / Math.max(visibleDisplayRows.length - 1, 1)) * plotWidth
  const hoveredX =
    activeIndex >= 0 && visibleDisplayRows.length
      ? getPlotX(activeIndex)
      : null
  const hoveredLowY =
    activeRow ? mapPriceToFrameY(activeRow.low, priceDomain.min, priceDomain.max, { top: chartFrame.top, height: plotHeight }) : null
  const hoveredHighY =
    activeRow ? mapPriceToFrameY(activeRow.high, priceDomain.min, priceDomain.max, { top: chartFrame.top, height: plotHeight }) : null
  const axisTicks = useMemo(() => {
    if (!visibleDisplayRows.length) {
      return []
    }
        const count = Math.min(6, visibleDisplayRows.length)
    const step = Math.max(1, Math.floor((visibleDisplayRows.length - 1) / Math.max(count - 1, 1)))
    const indices = new Set<number>([0, visibleDisplayRows.length - 1])
    for (let i = 1; i < count - 1; i += 1) {
      indices.add(Math.min(visibleDisplayRows.length - 1, i * step))
    }
    return Array.from(indices)
      .sort((a, b) => a - b)
      .map((index) => {
        const row = visibleDisplayRows[index]
        const date = dayjs(row.date)
        let label = ''
        if (timeframe === 'month') {
          label = date.format('YYYY-MM')
        } else if (timeframe === 'week') {
          label = date.format(index === 0 || date.month() === 0 ? 'YYYY/MM' : 'MM/DD')
        } else {
          label = date.format(index === 0 || date.month() === 0 ? 'YYYY/MM' : 'MM-DD')
        }
        return {
          index,
          label,
          x: getPlotX(index),
        }
      })
  }, [timeframe, visibleDisplayRows])
  const priceTicks = useMemo(() => {
    const steps = 5
    return Array.from({ length: steps }, (_, index) => {
      const ratio = index / Math.max(steps - 1, 1)
      const value = priceDomain.max - ((priceDomain.max - priceDomain.min) * ratio)
      return {
        value,
        y: chartFrame.top + plotHeight * ratio,
      }
    })
  }, [chartFrame.top, plotHeight, priceDomain.max, priceDomain.min])
  const draggingRangeLabel = useMemo(() => {
    if (interactionMode !== 'select' || dragStartX === null || dragCurrentX === null || !displayRowsRef.current.length) {
      return null
    }
    const currentRows = displayRowsRef.current
    const startIndex = clampDragIndex(dragStartX)
    const endIndex = clampDragIndex(dragCurrentX)
    const { from, to } = clampRange(currentRows.length, startIndex, endIndex)
    if (from === to) {
      return null
    }
    return `${dayjs(currentRows[from].startDate).format('YYYY-MM-DD')} -> ${dayjs(currentRows[to].endDate).format('YYYY-MM-DD')}`
  }, [dragCurrentX, dragStartX, interactionMode])
  return (
    <Card
      className="panel-card"
      title="行情图表"
      extra={
        <Space wrap size={6}>
          {(['day', 'week', 'month'] as Timeframe[]).map((option) => (
            <Button
              key={option}
              size="small"
              type={timeframe === option ? 'primary' : 'text'}
              onClick={() => setTimeframe(option)}
            >
              {timeframeLabels[option]}
            </Button>
          ))}
          <Tooltip title="放大当前视窗">
            <Button size="small" icon={<PlusOutlined />} onClick={() => zoomRange(0.76)} />
          </Tooltip>
          <Tooltip title="缩小当前视窗">
            <Button size="small" icon={<MinusOutlined />} onClick={() => zoomRange(1.28)} />
          </Tooltip>
          <Tooltip title="恢复当前周期默认视窗">
            <Button
              size="small"
              icon={<ExpandOutlined />}
              onClick={() => {
                const currentRows = displayRowsRef.current
                if (currentRows.length) {
                  applyVisibleRange(computeSpanRange(timeframe, currentRows.length))
                }
              }}
            />
          </Tooltip>
          <Tooltip title={interactionMode === 'select' ? '当前为框选模式，拖拽后自动退出' : '点击进入框选模式'}>
            <Button
              size="small"
              icon={<BorderOutlined />}
              type={interactionMode === 'select' ? 'primary' : 'default'}
              onClick={() =>
                setInteractionMode((current) => (current === 'select' ? 'browse' : 'select'))
              }
            >
              框选区间
            </Button>
          </Tooltip>
          <Tooltip title="清空区间选择">
            <Button
              size="small"
              icon={<CompressOutlined />}
              onClick={() => onRangeChange(null)}
              disabled={!range?.[0] || !range?.[1]}
            />
          </Tooltip>
          {([
            ['ma5', 'MA5'],
            ['ma10', 'MA10'],
            ['ma20', 'MA20'],
          ] as const).map(([key, label]) => (
            <Button
              key={key}
              size="small"
              type={overlayVisibility[key] ? 'default' : 'text'}
              onClick={() =>
                setOverlayVisibility((current) => ({
                  ...current,
                  [key]: !current[key],
                }))
              }
            >
              {label}
            </Button>
          ))}
          <Select<IndicatorKey[]>
            mode="multiple"
            size="small"
            value={selectedIndicators}
            onChange={(value) => setSelectedIndicators(value)}
            placeholder="选择下方指标"
            maxTagCount={2}
            style={{ minWidth: 220 }}
            options={[
              { label: '成交量', value: 'volume' },
              { label: 'MA', value: 'ma' },
              { label: 'RSI', value: 'rsi' },
              { label: 'BOLL', value: 'boll' },
              { label: 'KDJ', value: 'kdj' },
              { label: 'MACD', value: 'macd' },
            ]}
          />
          <Tag>{timeframeDescriptions[timeframe]}</Tag>
          <Tag color="processing">{selectionLabel}</Tag>
        </Space>
      }
    >
      {displayRows.length ? (
        <>
          <div
            ref={overlayRef}
            className={`chart-overlay ${isLoading ? 'chart-overlay--loading' : ''} ${interactionMode === 'select' ? 'chart-overlay--selecting' : ''}`}
            onContextMenu={(event) => event.preventDefault()}
            onWheel={handleOverlayWheel}
            onDoubleClick={handleDoubleClick}
            onPointerDown={handlePointerDown}
            onPointerMove={handlePointerMove}
            onPointerUp={handlePointerUp}
            onPointerLeave={handlePointerLeave}
          >
            <div className="chart-surface">
              <div ref={containerRef} className="chart-canvas-host" aria-hidden="true" />
              <svg viewBox={`0 0 ${mainChartWidth} ${mainChartHeight}`} className="chart-svg" preserveAspectRatio="none">
                <defs>
                  <linearGradient id="chartAreaFill" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="0%" stopColor="rgba(217, 45, 32, 0.14)" />
                    <stop offset="100%" stopColor="rgba(217, 45, 32, 0)" />
                  </linearGradient>
                </defs>
                {[0.2, 0.4, 0.6, 0.8].map((ratio) => (
                  <line
                    key={ratio}
                    x1={chartFrame.left}
                    y1={chartFrame.top + plotHeight * ratio}
                    x2={mainChartWidth - chartFrame.right}
                    y2={chartFrame.top + plotHeight * ratio}
                    className="chart-grid-line"
                  />
                ))}
                <line
                  x1={chartFrame.left}
                  y1={chartFrame.top + plotHeight}
                  x2={mainChartWidth - chartFrame.right}
                  y2={chartFrame.top + plotHeight}
                  className="chart-axis-line"
                />
                {priceTicks.map((tick) => (
                  <g key={`price-${tick.y}`}>
                    <line
                      x1={mainChartWidth - chartFrame.right}
                      y1={tick.y}
                      x2={mainChartWidth - chartFrame.right + 6}
                      y2={tick.y}
                      className="chart-axis-tick"
                    />
                    <text
                      x={mainChartWidth - 2}
                      y={tick.y + 4}
                      textAnchor="end"
                      className="chart-axis-label"
                    >
                      {formatNumber(tick.value)}
                    </text>
                  </g>
                ))}
                {priceLineFramed ? (
                  <>
                    <polyline
                      points={`${priceLineFramed} ${chartFrame.left + plotWidth},${chartFrame.top + plotHeight} ${chartFrame.left},${chartFrame.top + plotHeight}`}
                      className="chart-area-fill"
                    />
                    <polyline points={priceLineFramed} className="chart-price-line" />
                  </>
                ) : null}
                {bollUpperLineFramed ? <polyline points={bollUpperLineFramed} className="chart-overlay-line chart-overlay-line--boll-upper" /> : null}
                {bollMiddleLineFramed ? <polyline points={bollMiddleLineFramed} className="chart-overlay-line chart-overlay-line--boll-middle" /> : null}
                {bollLowerLineFramed ? <polyline points={bollLowerLineFramed} className="chart-overlay-line chart-overlay-line--boll-lower" /> : null}
                {overlayVisibility.ma5 && ma5LineMain ? (
                  <polyline points={ma5LineFramed} className="chart-overlay-line chart-overlay-line--ma5" />
                ) : null}
                {overlayVisibility.ma10 && ma10LineMain ? (
                  <polyline points={ma10LineFramed} className="chart-overlay-line chart-overlay-line--ma10" />
                ) : null}
                {overlayVisibility.ma20 && ma20LineMain ? (
                  <polyline points={ma20LineFramed} className="chart-overlay-line chart-overlay-line--ma20" />
                ) : null}
                {visibleDisplayRows.map((item, index) => {
                  const x = getPlotX(index)
                  const openY = mapPriceToFrameY(item.open, priceDomain.min, priceDomain.max, { top: chartFrame.top, height: plotHeight })
                  const closeY = mapPriceToFrameY(item.close, priceDomain.min, priceDomain.max, { top: chartFrame.top, height: plotHeight })
                  const highY = mapPriceToFrameY(item.high, priceDomain.min, priceDomain.max, { top: chartFrame.top, height: plotHeight })
                  const lowY = mapPriceToFrameY(item.low, priceDomain.min, priceDomain.max, { top: chartFrame.top, height: plotHeight })
                  const bodyTop = Math.min(openY, closeY)
                  const bodyHeight = Math.max(Math.abs(openY - closeY), 2)
                  const isUp = item.close >= item.open
                  return (
                    <g key={`${item.date}-candle`}>
                      <line
                        x1={x}
                        y1={highY}
                        x2={x}
                        y2={lowY}
                        className={isUp ? 'chart-candle-wick chart-candle-wick--up' : 'chart-candle-wick chart-candle-wick--down'}
                        style={{ stroke: isUp ? riseColor : fallColor }}
                      />
                      <rect
                        x={x - candleBodyWidth / 2}
                        y={bodyTop}
                        width={candleBodyWidth}
                        height={bodyHeight}
                        rx="1.5"
                        className={isUp ? 'chart-candle-body chart-candle-body--up' : 'chart-candle-body chart-candle-body--down'}
                        style={{ fill: isUp ? riseColor : fallColor }}
                      />
                    </g>
                  )
                })}
                {hoveredX !== null && hoveredLowY !== null && hoveredHighY !== null ? (
                  <>
                    <line x1={hoveredX} y1={chartFrame.top} x2={hoveredX} y2={chartFrame.top + plotHeight} className="chart-crosshair-line" style={{ stroke: crosshairColor }} />
                    <line x1={chartFrame.left} y1={hoveredLowY} x2={mainChartWidth - chartFrame.right} y2={hoveredLowY} className="chart-crosshair-line" style={{ stroke: crosshairColor }} />
                    <circle cx={hoveredX} cy={hoveredHighY} r="3.5" className="chart-crosshair-dot" style={{ fill: riseColor }} />
                  </>
                ) : null}
                {axisTicks.map((tick) => (
                  <g key={`${tick.label}-${tick.index}`}>
                    <line
                      x1={tick.x}
                      y1={chartFrame.top + plotHeight}
                      x2={tick.x}
                      y2={chartFrame.top + plotHeight + 6}
                      className="chart-axis-tick"
                    />
                    <text
                      x={tick.x}
                      y={chartFrame.top + plotHeight + 20}
                      textAnchor="middle"
                      className="chart-axis-label"
                    >
                      {tick.label}
                    </text>
                  </g>
                ))}
              </svg>
            </div>
            <div className="chart-hint">
              <Space size={8} wrap>
                <span>
                  <BorderOutlined /> 点击“框选区间”后拖拽
                </span>
                <span>
                  <PlusOutlined /> <MinusOutlined /> 缩放
                </span>
                <span>双击重置</span>
                <span>{timeframeLabels[timeframe]}</span>
              </Space>
            </div>
            {selectionStyle ? (
              <div
                className="chart-selection"
                style={{
                  left: selectionStyle.left,
                  width: selectionStyle.width,
                }}
              />
            ) : null}
            {selectionStyle && draggingRangeLabel ? (
              <div
                className="chart-selection-label"
                style={{
                  left: selectionStyle.left,
                }}
              >
                {draggingRangeLabel}
              </div>
            ) : null}
          </div>
          {stats ? (
            <>
              <div className="indicator-grid">
                {selectedIndicators.includes('volume') ? (
                <div className="indicator-card">
                  <div className="indicator-head">
                    <strong>成交量</strong>
                    <span>最新 {formatVolume(visibleDisplayRows.at(-1)?.volume)}</span>
                  </div>
                  <svg viewBox="0 0 520 88" className="indicator-svg" preserveAspectRatio="none">
                    <line x1="0" y1="88" x2="520" y2="88" className="indicator-guide indicator-guide--mid" />
                    {visibleDisplayRows.map((item, index) => {
                      const x = (index / Math.max(visibleDisplayRows.length - 1, 1)) * 520
                      const height = visibleVolumeMax === 0 ? 0 : (item.volume / visibleVolumeMax) * 80
                      const y = 88 - height
                      const isUp = item.close >= item.open
                      return (
                        <rect
                          key={`${item.date}-volume`}
                          x={x}
                          y={y}
                          width={Math.max(520 / Math.max(visibleDisplayRows.length, 40) - 1, 2)}
                          height={height}
                          className={isUp ? 'indicator-bar indicator-bar--up' : 'indicator-bar indicator-bar--down'}
                        />
                      )
                    })}
                  </svg>
                  <div className="indicator-foot">
                    <span>最大 {formatVolume(visibleVolumeMax)}</span>
                    <span>{visibleDisplayRows[0]?.date ?? '--'}</span>
                    <span>{visibleDisplayRows.at(-1)?.date ?? '--'}</span>
                  </div>
                </div>
                ) : null}

                {selectedIndicators.includes('ma') ? (
                <div className="indicator-card">
                  <div className="indicator-head">
                    <strong>均线 MA (5, 10, 20)</strong>
                    <span>
                      MA5 {formatNumber(latestMA5 ?? undefined)} / MA10 {formatNumber(latestMA10 ?? undefined)} / MA20 {formatNumber(latestMA20 ?? undefined)}
                    </span>
                  </div>
                  <svg viewBox="0 0 520 88" className="indicator-svg" preserveAspectRatio="none">
                    {priceLine ? (
                      <polyline
                        fill="none"
                        points={priceLine}
                        className="indicator-line indicator-line--price"
                      />
                    ) : null}
                    {ma5Line ? (
                      <polyline
                        fill="none"
                        points={ma5Line}
                        className="indicator-line indicator-line--ma5"
                      />
                    ) : null}
                    {ma10Line ? (
                      <polyline
                        fill="none"
                        points={ma10Line}
                        className="indicator-line indicator-line--ma10"
                      />
                    ) : null}
                    {ma20Line ? (
                      <polyline
                        fill="none"
                        points={ma20Line}
                        className="indicator-line indicator-line--ma20"
                      />
                    ) : null}
                  </svg>
                  <div className="indicator-foot">
                    <span>收盘 {formatNumber(visibleDisplayRows.at(-1)?.close)}</span>
                    <span>{visibleDisplayRows[0]?.date ?? '--'}</span>
                    <span>{visibleDisplayRows.at(-1)?.date ?? '--'}</span>
                  </div>
                </div>
                ) : null}

                {selectedIndicators.includes('rsi') ? (
                <div className="indicator-card">
                  <div className="indicator-head">
                    <strong>RSI (14)</strong>
                    <span>{formatSigned(latestRSI)}</span>
                  </div>
                  <svg viewBox="0 0 520 88" className="indicator-svg" preserveAspectRatio="none">
                    <line x1="0" y1="17.6" x2="520" y2="17.6" className="indicator-guide" />
                    <line x1="0" y1="44" x2="520" y2="44" className="indicator-guide indicator-guide--mid" />
                    <line x1="0" y1="70.4" x2="520" y2="70.4" className="indicator-guide" />
                    {rsiPolyline ? (
                      <polyline
                        fill="none"
                        points={rsiPolyline}
                        className="indicator-line indicator-line--rsi"
                      />
                    ) : null}
                  </svg>
                  <div className="indicator-foot">
                    <span>70</span>
                    <span>50</span>
                    <span>30</span>
                  </div>
                </div>
                ) : null}

                {selectedIndicators.includes('boll') ? (
                <div className="indicator-card">
                  <div className="indicator-head">
                    <strong>BOLL (20, 2)</strong>
                    <span>
                      MID {formatNumber(latestBOLL?.middle ?? undefined)} / UP {formatNumber(latestBOLL?.upper ?? undefined)} / LOW {formatNumber(latestBOLL?.lower ?? undefined)}
                    </span>
                  </div>
                  <svg viewBox="0 0 520 88" className="indicator-svg" preserveAspectRatio="none">
                    {bollUpperLine ? (
                      <polyline
                        fill="none"
                        points={bollUpperLine}
                        className="indicator-line indicator-line--boll-upper"
                      />
                    ) : null}
                    {bollMiddleLine ? (
                      <polyline
                        fill="none"
                        points={bollMiddleLine}
                        className="indicator-line indicator-line--boll-middle"
                      />
                    ) : null}
                    {bollLowerLine ? (
                      <polyline
                        fill="none"
                        points={bollLowerLine}
                        className="indicator-line indicator-line--boll-lower"
                      />
                    ) : null}
                  </svg>
                  <div className="indicator-foot">
                    <span>
                      带宽 {latestBOLL && latestBOLL.upper !== null && latestBOLL.lower !== null
                        ? formatNumber(latestBOLL.upper - latestBOLL.lower)
                        : '--'}
                    </span>
                    <span>{visibleDisplayRows[0]?.date ?? '--'}</span>
                    <span>{visibleDisplayRows.at(-1)?.date ?? '--'}</span>
                  </div>
                </div>
                ) : null}

                {selectedIndicators.includes('kdj') ? (
                <div className="indicator-card">
                  <div className="indicator-head">
                    <strong>KDJ (9, 3, 3)</strong>
                    <span>
                      K {formatSigned(latestKDJ?.k)} / D {formatSigned(latestKDJ?.d)} / J {formatSigned(latestKDJ?.j)}
                    </span>
                  </div>
                  <svg viewBox="0 0 520 88" className="indicator-svg" preserveAspectRatio="none">
                    <line x1="0" y1="17.6" x2="520" y2="17.6" className="indicator-guide" />
                    <line x1="0" y1="44" x2="520" y2="44" className="indicator-guide indicator-guide--mid" />
                    <line x1="0" y1="70.4" x2="520" y2="70.4" className="indicator-guide" />
                    {kLine ? (
                      <polyline
                        fill="none"
                        points={kLine}
                        className="indicator-line indicator-line--k"
                      />
                    ) : null}
                    {dLine ? (
                      <polyline
                        fill="none"
                        points={dLine}
                        className="indicator-line indicator-line--d"
                      />
                    ) : null}
                    {jLine ? (
                      <polyline
                        fill="none"
                        points={jLine}
                        className="indicator-line indicator-line--j"
                      />
                    ) : null}
                  </svg>
                  <div className="indicator-foot">
                    <span>80</span>
                    <span>50</span>
                    <span>20</span>
                  </div>
                </div>
                ) : null}

                {selectedIndicators.includes('macd') ? (
                <div className="indicator-card">
                  <div className="indicator-head">
                    <strong>MACD (12, 26, 9)</strong>
                    <span>
                      DIF {formatSigned(latestMACD?.macd)} / DEA {formatSigned(latestMACD?.signal)}
                    </span>
                  </div>
                  <svg viewBox="0 0 520 88" className="indicator-svg" preserveAspectRatio="none">
                    <line x1="0" y1="44" x2="520" y2="44" className="indicator-guide indicator-guide--mid" />
                    {visibleMACD.map((item, index) => {
                      if (item.histogram === null) {
                        return null
                      }
                      const x = (index / Math.max(visibleMACD.length - 1, 1)) * 520
                      const barHeight = Math.min(Math.abs(item.histogram) * 120, 40)
                      const y = item.histogram >= 0 ? 44 - barHeight : 44
                      return (
                        <rect
                          key={`${item.date}-hist`}
                          x={x}
                          y={y}
                          width={Math.max(520 / Math.max(visibleMACD.length, 40) - 1, 2)}
                          height={barHeight}
                          className={
                            item.histogram >= 0
                              ? 'indicator-bar indicator-bar--up'
                              : 'indicator-bar indicator-bar--down'
                          }
                        />
                      )
                    })}
                    {macdLine ? (
                      <polyline
                        fill="none"
                        points={macdLine}
                        className="indicator-line indicator-line--macd"
                      />
                    ) : null}
                    {signalLine ? (
                      <polyline
                        fill="none"
                        points={signalLine}
                        className="indicator-line indicator-line--signal"
                      />
                    ) : null}
                  </svg>
                  <div className="indicator-foot">
                    <span>Histogram {formatSigned(latestMACD?.histogram)}</span>
                    <span>{visibleDisplayRows[0]?.date ?? '--'}</span>
                    <span>{visibleDisplayRows.at(-1)?.date ?? '--'}</span>
                  </div>
                </div>
                ) : null}
              </div>
            </>
          ) : null}
        </>
      ) : (
        <Empty description="Select a symbol to render chart data." />
      )}
    </Card>
  )
}
