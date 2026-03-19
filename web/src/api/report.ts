import axios from 'axios'

export interface HostReport {
  host: string
  uptime_pct: number
  cpu_avg: number
  cpu_max: number
  mem_avg: number
  mem_max: number
  disk_max: number
}

export interface MonthlyReport {
  year: number
  month: number
  hosts: HostReport[]
}

export async function getReportPreview(year?: number, month?: number): Promise<MonthlyReport> {
  const params: Record<string, number> = {}
  if (year) params.year = year
  if (month) params.month = month
  const res = await axios.get('/api/report/preview', { params })
  return res.data
}

export async function sendReport(year?: number, month?: number): Promise<void> {
  await axios.post('/api/report/send', { year, month })
}
