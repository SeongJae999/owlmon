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
  rack: string // 빈 값이면 랙에 넣지 않은 장비
  rack_unit: number // 장비 맨 아래 칸 (1 = 랙 맨 아래)
  rack_size: number // 차지하는 칸 수
  updated_at: string
}

export const unitRange = (unit: number, size: number) => size > 1 ? `${unit}~${unit + size - 1}U` : `${unit}U`

// 서버 db.LocationText와 같은 문구 — 알림 메일과 화면이 같은 위치를 보여 주게
export function locationText(a: Pick<Asset, 'location' | 'rack' | 'rack_unit' | 'rack_size'> | undefined): string {
  if (!a) return ''
  return [a.location.trim(), a.rack && a.rack_unit > 0 && `${a.rack} ${unitRange(a.rack_unit, a.rack_size)}`].filter(Boolean).join(' · ')
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

// 서버별 실제 수집 경과 초 — 서버 시계로 잰 값이라 에이전트·브라우저 시계와 무관
export async function fetchHostFreshness(): Promise<Record<string, { age_sec: number; interval_sec?: number; ip?: string }>> {
  const res = await axios.get<Record<string, { age_sec: number; interval_sec?: number; ip?: string }>>('/api/hosts/freshness')
  return res.data ?? {}
}

export async function fetchUptime(): Promise<Record<string, number>> {
  try {
    const res = await axios.get<Record<string, number>>('/api/uptime')
    return res.data
  } catch {
    return {}
  }
}
