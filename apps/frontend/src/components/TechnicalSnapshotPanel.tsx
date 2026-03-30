import { Card, Empty, Flex, Space, Tag, Typography } from 'antd'
import dayjs, { type Dayjs } from 'dayjs'
import { useMemo } from 'react'
import type { OHLCRow } from '../types/api'

const { Text } = Typography

interface TechnicalSnapshotPanelProps {
  range: [Dayjs | null, Dayjs | null] | null
  rows: OHLCRow[]
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

function computeRSI(rows: OHLCRow[], period = 14) {
  if (rows.length <= period) {
    return null
  }

  const closes = rows.map((row) => row.close)
  let gains = 0
  let losses = 0
  for (let i = 1; i <= period; i += 1) {
    const delta = closes[i] - closes[i - 1]
    if (delta >= 0) gains += delta
    if (delta < 0) losses += Math.abs(delta)
  }

  let averageGain = gains / period
  let averageLoss = losses / period
  let latest = averageLoss === 0 ? 100 : 100 - (100 / (1 + averageGain / averageLoss))

  for (let i = period + 1; i < closes.length; i += 1) {
    const delta = closes[i] - closes[i - 1]
    const gain = delta > 0 ? delta : 0
    const loss = delta < 0 ? Math.abs(delta) : 0
    averageGain = ((averageGain * (period - 1)) + gain) / period
    averageLoss = ((averageLoss * (period - 1)) + loss) / period
    latest = averageLoss === 0 ? 100 : 100 - (100 / (1 + averageGain / averageLoss))
  }

  return latest
}

function computeMACD(rows: OHLCRow[]) {
  if (rows.length < 26) {
    return null
  }

  const closes = rows.map((row) => row.close)
  const ema12 = computeEMA(closes, 12)
  const ema26 = computeEMA(closes, 26)
  const macdValues = closes.map((_, index) =>
    ema12[index] !== null && ema26[index] !== null ? ema12[index]! - ema26[index]! : null,
  )
  const signalValues = computeEMA(macdValues.map((value) => value ?? 0), 9)
  const lastIndex = macdValues.length - 1
  const macd = macdValues[lastIndex]
  const signal = signalValues[lastIndex]

  if (macd === null || signal === null) {
    return null
  }

  return {
    macd,
    signal,
    histogram: macd - signal,
  }
}

function computeMA(rows: OHLCRow[], period: number) {
  if (rows.length < period) {
    return null
  }
  const slice = rows.slice(-period)
  return slice.reduce((sum, row) => sum + row.close, 0) / period
}

function computeKDJ(rows: OHLCRow[], period = 9) {
  if (rows.length < period) {
    return null
  }

  let k = 50
  let d = 50
  for (let i = period - 1; i < rows.length; i += 1) {
    const window = rows.slice(i - period + 1, i + 1)
    const low = Math.min(...window.map((item) => item.low))
    const high = Math.max(...window.map((item) => item.high))
    const rsv = high === low ? 50 : ((rows[i].close - low) / (high - low)) * 100
    k = (2 / 3) * k + (1 / 3) * rsv
    d = (2 / 3) * d + (1 / 3) * k
  }

  return {
    k,
    d,
    j: (3 * k) - (2 * d),
  }
}

function computeBOLL(rows: OHLCRow[], period = 20, multiplier = 2) {
  if (rows.length < period) {
    return null
  }
  const window = rows.slice(-period)
  const middle = window.reduce((sum, row) => sum + row.close, 0) / period
  const variance = window.reduce((sum, row) => sum + ((row.close - middle) ** 2), 0) / period
  const deviation = Math.sqrt(variance)
  return {
    middle,
    upper: middle + deviation * multiplier,
    lower: middle - deviation * multiplier,
  }
}

function formatSigned(value?: number | null) {
  if (value === undefined || value === null || Number.isNaN(value)) {
    return '--'
  }
  const sign = value > 0 ? '+' : ''
  return `${sign}${value.toFixed(2)}`
}

function describeRSI(value: number | null) {
  if (value === null) return '数据不足'
  if (value >= 70) return '偏强，接近超买'
  if (value <= 30) return '偏弱，接近超卖'
  if (value >= 55) return '强势区'
  if (value <= 45) return '弱势区'
  return '中性震荡'
}

function describeMACD(macd?: { macd: number; signal: number; histogram: number } | null) {
  if (!macd) return '数据不足'
  if (macd.macd > macd.signal && macd.histogram > 0) return '多头动能占优'
  if (macd.macd < macd.signal && macd.histogram < 0) return '空头动能占优'
  return '动能纠结，等待方向'
}

function describeTechnicalBias(snapshot: {
  latestClose: number
  ma5: number | null
  ma10: number | null
  ma20: number | null
  rsi: number | null
  macd: { macd: number; signal: number; histogram: number } | null
  boll: { middle: number; upper: number; lower: number } | null
}) {
  const trendUp = snapshot.ma5 !== null && snapshot.ma10 !== null && snapshot.ma20 !== null
    && snapshot.ma5 >= snapshot.ma10
    && snapshot.ma10 >= snapshot.ma20
  const trendDown = snapshot.ma5 !== null && snapshot.ma10 !== null && snapshot.ma20 !== null
    && snapshot.ma5 <= snapshot.ma10
    && snapshot.ma10 <= snapshot.ma20

  if (trendUp && snapshot.macd && snapshot.macd.histogram >= 0 && (snapshot.rsi ?? 50) >= 50) {
    return '偏多，趋势和动能一致'
  }
  if (trendDown && snapshot.macd && snapshot.macd.histogram <= 0 && (snapshot.rsi ?? 50) <= 50) {
    return '偏空，趋势和动能一致'
  }
  if (snapshot.boll && snapshot.latestClose >= snapshot.boll.upper) {
    return '强势冲击上轨，短线波动偏大'
  }
  if (snapshot.boll && snapshot.latestClose <= snapshot.boll.lower) {
    return '弱势贴近下轨，注意惯性下探'
  }
  return '中性，等待价格和动能重新共振'
}

function computeKeyLevels(rows: OHLCRow[]) {
  if (!rows.length) {
    return null
  }

  const latest = rows[rows.length - 1]
  const recent = rows.slice(-20)
  const ma10 = computeMA(rows, 10)
  const ma20 = computeMA(rows, 20)
  const boll = computeBOLL(rows)
  const recentLow = Math.min(...recent.map((row) => row.low))
  const recentHigh = Math.max(...recent.map((row) => row.high))
  const supports = [recentLow, ma10, ma20, boll?.lower]
    .filter((value): value is number => value !== null && value !== undefined)
    .filter((value) => value <= latest.close * 1.02)
    .sort((a, b) => b - a)
  const resistances = [recentHigh, ma10, ma20, boll?.upper]
    .filter((value): value is number => value !== null && value !== undefined)
    .filter((value) => value >= latest.close * 0.98)
    .sort((a, b) => a - b)

  const support = supports[0] ?? recentLow
  const pressure = resistances[0] ?? recentHigh
  const risk = Math.min(support, boll?.lower ?? support, recentLow) * 0.985

  const supportReasons = [
    ma10 ? `MA10 ${ma10.toFixed(2)}` : null,
    ma20 ? `MA20 ${ma20.toFixed(2)}` : null,
    boll?.lower ? `BOLL下轨 ${boll.lower.toFixed(2)}` : null,
    `近20根低点 ${recentLow.toFixed(2)}`,
  ].filter(Boolean) as string[]
  const pressureReasons = [
    boll?.upper ? `BOLL上轨 ${boll.upper.toFixed(2)}` : null,
    `近20根高点 ${recentHigh.toFixed(2)}`,
  ].filter(Boolean) as string[]

  return {
    support,
    pressure,
    risk,
    supportReason: supportReasons.join(' / '),
    pressureReason: pressureReasons.join(' / '),
    riskReason: '当前版本未接入新闻源；风险位仅基于 MA、BOLL 和近20根价格结构。',
  }
}

export function TechnicalSnapshotPanel({ range, rows }: TechnicalSnapshotPanelProps) {
  const technicalRows = useMemo(() => {
    if (!range?.[0] || !range?.[1]) {
      return rows
    }
    const start = dayjs(range[0])
    const end = dayjs(range[1])
    return rows.filter((row) => {
      const date = dayjs(row.date)
      return !date.isBefore(start) && !date.isAfter(end)
    })
  }, [range, rows])

  const technicalSnapshot = useMemo(() => {
    if (!technicalRows.length) {
      return null
    }

    const latest = technicalRows[technicalRows.length - 1]
    const previous = technicalRows.length > 1 ? technicalRows[technicalRows.length - 2] : null
    const changePct = previous && previous.close !== 0
      ? ((latest.close - previous.close) / previous.close) * 100
      : null

    return {
      latestClose: latest.close,
      changePct,
      candles: technicalRows.length,
      rsi: computeRSI(technicalRows),
      macd: computeMACD(technicalRows),
      ma5: computeMA(technicalRows, 5),
      ma10: computeMA(technicalRows, 10),
      ma20: computeMA(technicalRows, 20),
      kdj: computeKDJ(technicalRows),
      boll: computeBOLL(technicalRows),
    }
  }, [technicalRows])

  const keyLevels = useMemo(() => computeKeyLevels(technicalRows), [technicalRows])

  return (
    <Card className="panel-card" title="Technical Snapshot" styles={{ body: { padding: 14 } }}>
      {technicalSnapshot ? (
        <Space direction="vertical" size={10} style={{ width: '100%' }}>
          <Flex justify="space-between" align="center" wrap="wrap" gap={10}>
            <div className="technical-summary">
              <strong>技术面结论</strong>
              <span>{describeTechnicalBias(technicalSnapshot)}</span>
            </div>
            <Space wrap size={6}>
              <Tag color="gold">收盘 {technicalSnapshot.latestClose.toFixed(2)}</Tag>
              <Tag color={technicalSnapshot.changePct !== null && technicalSnapshot.changePct >= 0 ? 'green' : 'red'}>
                涨跌 {formatSigned(technicalSnapshot.changePct)}%
              </Tag>
              <Tag color="blue">Candles {technicalSnapshot.candles}</Tag>
              <Tag color="purple">RSI {formatSigned(technicalSnapshot.rsi)}</Tag>
              <Tag color="cyan">MACD {formatSigned(technicalSnapshot.macd?.macd)}</Tag>
            </Space>
          </Flex>
          {keyLevels ? (
            <div className="technical-levels">
              <div className="technical-level">
                <span>支撑位</span>
                <strong>{keyLevels.support.toFixed(2)}</strong>
                <small>{keyLevels.supportReason}</small>
              </div>
              <div className="technical-level">
                <span>压力位</span>
                <strong>{keyLevels.pressure.toFixed(2)}</strong>
                <small>{keyLevels.pressureReason}</small>
              </div>
              <div className="technical-level">
                <span>风险位</span>
                <strong>{keyLevels.risk.toFixed(2)}</strong>
                <small>{keyLevels.riskReason}</small>
              </div>
            </div>
          ) : null}
          <Text type="secondary">
            RSI: {describeRSI(technicalSnapshot.rsi)}；MACD: {describeMACD(technicalSnapshot.macd)}；BOLL 中轨 {formatSigned(technicalSnapshot.boll?.middle)}
          </Text>
        </Space>
      ) : (
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="当前区间数据不足，暂时无法计算指标。" />
      )}
    </Card>
  )
}
