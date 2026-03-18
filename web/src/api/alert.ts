import axios from 'axios'

export interface AlertConfig {
  enabled: boolean
  recipients: string[]
  cpu_threshold: number
  mem_threshold: number
  disk_warn: number
  disk_crit: number
}

export async function getAlertConfig(): Promise<AlertConfig> {
  const res = await axios.get('/api/alert/config')
  return res.data
}

export async function setAlertConfig(cfg: AlertConfig): Promise<AlertConfig> {
  const res = await axios.post('/api/alert/config', cfg)
  return res.data
}
