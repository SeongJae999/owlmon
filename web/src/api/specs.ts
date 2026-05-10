import axios from 'axios'

// 에이전트가 보고한 호스트 하드웨어/OS 스펙
export interface DiskInfo {
  name: string
  size_bytes: number
  rotational: boolean
  model: string
}

export interface NetworkInfo {
  name: string
  mac: string
  ipv4: string[]
}

export interface HostSpec {
  host_name: string
  cpu_model: string
  cpu_cores: number
  cpu_sockets: number
  memory_total_bytes: number
  disks: DiskInfo[]
  networks: NetworkInfo[]
  os_pretty_name: string
  kernel_version: string
  virtualization: string
  arch: string
  collected_at: string
  updated_at: string
}

export async function listHostSpecs(): Promise<HostSpec[]> {
  const res = await axios.get('/api/agent/specs')
  return res.data ?? []
}

export async function getHostSpec(host: string): Promise<HostSpec | null> {
  try {
    const res = await axios.get(`/api/agent/specs/${encodeURIComponent(host)}`)
    return res.data
  } catch (err: any) {
    if (err?.response?.status === 404) return null
    throw err
  }
}

// ─── 표시용 헬퍼 ──────────────────────────────────────────

export function formatBytes(n: number): string {
  if (!n || n <= 0) return '—'
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  let i = 0
  let v = n
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(v >= 100 ? 0 : v >= 10 ? 1 : 2)} ${units[i]}`
}

// gopsutil은 KVM 호스트(베어메탈이 다른 VM 돌리는 경우)도 "kvm"으로 표시함.
// 의미를 정확히 보여주기 위해 보정.
export function formatVirtualization(v: string): string {
  const x = (v || '').toLowerCase()
  if (!x || x === 'none' || x === '') return '베어메탈'
  if (x === 'kvm') return '베어메탈 (KVM 호스트)'
  if (x === 'docker') return '컨테이너 (Docker)'
  if (x === 'lxc') return '컨테이너 (LXC)'
  return `VM (${x})`
}

// CPU 모델명을 짧게 (Overview 요약용)
export function shortCPU(model: string): string {
  if (!model) return ''
  // "Intel(R) Core(TM) i5-14500" → "i5-14500"
  // "Intel(R) Xeon(R) CPU E5-2665 0 @ 2.40GHz" → "Xeon E5-2665"
  return model
    .replace(/Intel\(R\)|AMD|\(TM\)|\(R\)|Core|CPU/gi, '')
    .replace(/@.*$/, '')
    .replace(/\s+/g, ' ')
    .trim()
}

// 디스크 배열을 짧게 요약: "SSD 2개·HDD 1개 (5.9 TB)"
export function summarizeDisks(disks: { rotational: boolean; size_bytes: number }[]): string {
  if (!disks?.length) return '—'
  const ssd = disks.filter((d) => !d.rotational)
  const hdd = disks.filter((d) => d.rotational)
  const total = disks.reduce((s, d) => s + (d.size_bytes || 0), 0)

  const parts: string[] = []
  if (ssd.length) parts.push(`SSD ${ssd.length}개`)
  if (hdd.length) parts.push(`HDD ${hdd.length}개`)
  return `${parts.join('·')} (${formatBytes(total)})`
}
