import axios from 'axios'

export interface Asset {
  id: number
  host_name: string
  ip: string
  location: string
  description: string
  purchase_date: string   // "YYYY-MM-DD" 또는 ""
  warranty_expires: string // "YYYY-MM-DD" 또는 ""
  notes: string
  updated_at: string
}

export async function fetchAssets(): Promise<Asset[]> {
  const res = await axios.get<Asset[]>('/api/assets')
  return res.data
}

export async function upsertAsset(asset: Omit<Asset, 'id' | 'updated_at'>): Promise<Asset> {
  const res = await axios.put<Asset>('/api/assets', asset)
  return res.data
}

export async function deleteAsset(id: number): Promise<void> {
  await axios.delete(`/api/assets/${id}`)
}

export async function fetchUptime(): Promise<Record<string, number>> {
  try {
    const res = await axios.get<Record<string, number>>('/api/uptime')
    return res.data
  } catch {
    return {}
  }
}
