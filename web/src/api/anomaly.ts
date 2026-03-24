import axios from 'axios'

export interface AnomalyItem {
  host: string
  metric: string
  value: number
  z_score: number
  mean: number
  std_dev: number
  severity: string
  message: string
  detected_at: string
}

export interface DiskPrediction {
  host: string
  mountpoint: string
  current: number
  slope: number
  days_left: number
  r2: number
  message: string
}

export interface AnomalyResponse {
  anomalies: AnomalyItem[]
  disk_predictions: DiskPrediction[]
  stats: {
    tracked_metrics: number
    active_anomalies: number
  }
}

export async function getAnomalyData(): Promise<AnomalyResponse> {
  const res = await axios.get('/api/anomaly')
  return res.data
}
