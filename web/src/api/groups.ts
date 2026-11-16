import axios from 'axios'

export type MemberKind = 'server' | 'snmp' | 'synthetic' | 'ssl' | 'dpm'

export interface DeviceGroup {
  id: number
  parent_id: number | null
  name: string
  sort: number
  ip_ranges: string[] // 서버가 1.2.3.4 / 1.2.3.0/24 / 1.2.3.4-1.2.3.9 꼴로 정리해 둠
}

export interface GroupMember {
  kind: MemberKind
  ref: string
  group_id: number
}

export interface GroupTree {
  groups: DeviceGroup[]
  members: GroupMember[]
}

export async function fetchGroups(): Promise<GroupTree> {
  const res = await axios.get<GroupTree>('/api/groups')
  return { groups: res.data?.groups ?? [], members: res.data?.members ?? [] }
}

export async function createGroup(g: { name: string; parent_id: number | null; ip_ranges?: string[] }): Promise<DeviceGroup> {
  const res = await axios.post<DeviceGroup>('/api/groups', { ip_ranges: [], ...g, sort: 0 })
  return res.data
}

export async function updateGroup(g: DeviceGroup): Promise<void> {
  await axios.put(`/api/groups/${g.id}`, g)
}

export async function deleteGroup(id: number): Promise<void> {
  await axios.delete(`/api/groups/${id}`)
}

export async function assignGroup(groupId: number | null, items: { kind: MemberKind; ref: string }[]): Promise<void> {
  await axios.put('/api/groups/members', { group_id: groupId, items })
}

// 화면 타일 key → 소속 키. 서버는 이름(DB에 서버 표가 없음), SSL은 도메인, 나머지는 각 표의 id라 이름을 바꿔도 소속이 유지됨
export function memberOf(tileKey: string, name: string): { kind: MemberKind; ref: string } | null {
  const [p, rest] = [tileKey.slice(0, 2), tileKey.slice(2)]
  switch (p) {
    case 'h-': return { kind: 'server', ref: rest }
    case 'n-': return { kind: 'snmp', ref: rest }
    case 'w-': return { kind: 'synthetic', ref: rest }
    case 'd-': return { kind: 'dpm', ref: rest }
    case 's-': return { kind: 'ssl', ref: name }
  }
  return null
}

export const memberKey = (m: { kind: string; ref: string }) => `${m.kind}:${m.ref}`

export const UNGROUPED = 'none'

export interface GroupIndex {
  byId: Map<number, DeviceGroup>
  children: Map<number | null, DeviceGroup[]>
  groupOf: Map<string, number> // memberKey → group id
  // id와 그 아래 모든 그룹 — 상위 그룹을 고르면 하위 장비까지 보여야 함
  subtree: (id: number) => Set<number>
  path: (id: number) => string[]
  byIP: (ip: string | undefined) => number | undefined
}

const ip4 = (s: string): number | null => {
  const p = s.split('.')
  if (p.length !== 4 || p.some(x => !/^\d{1,3}$/.test(x) || +x > 255)) return null
  return ((+p[0] << 24) >>> 0) + (+p[1] << 16) + (+p[2] << 8) + +p[3]
}

export function parseRange(r: string): [number, number] | null {
  const [a, b] = r.split('-')
  if (b !== undefined) { const lo = ip4(a), hi = ip4(b); return lo != null && hi != null && lo <= hi ? [lo, hi] : null }
  const [addr, bits] = r.split('/')
  const lo = ip4(addr)
  if (lo == null) return null
  if (bits === undefined) return [lo, lo]
  const n = Number(bits)
  if (!Number.isInteger(n) || n < 0 || n > 32) return null
  const size = 2 ** (32 - n)
  const base = lo - (lo % size)
  return [base, base + size - 1]
}

// 직접 지정한 소속이 우선, 없으면 IP 대역 — 여러 대역에 걸리면 가장 좁은 대역(본사 /16보다 3층 /24)
export function tileGroup(idx: GroupIndex, t: { key: string; name: string; ip?: string }): { id: number; auto: boolean } | undefined {
  const m = memberOf(t.key, t.name)
  const g = m ? idx.groupOf.get(memberKey(m)) : undefined
  if (g != null && idx.byId.has(g)) return { id: g, auto: false }
  const a = idx.byIP(t.ip)
  return a != null ? { id: a, auto: true } : undefined
}

export function indexGroups(t: GroupTree | undefined): GroupIndex {
  const byId = new Map<number, DeviceGroup>()
  const children = new Map<number | null, DeviceGroup[]>()
  for (const g of t?.groups ?? []) {
    byId.set(g.id, g)
    const list = children.get(g.parent_id) ?? []
    list.push(g)
    children.set(g.parent_id, list)
  }
  const groupOf = new Map((t?.members ?? []).map(m => [memberKey(m), m.group_id]))
  const subCache = new Map<number, Set<number>>()
  const subtree = (id: number) => {
    let s = subCache.get(id)
    if (!s) {
      s = new Set([id])
      const stack = [id]
      while (stack.length) for (const c of children.get(stack.pop()!) ?? []) if (!s.has(c.id)) { s.add(c.id); stack.push(c.id) }
      subCache.set(id, s)
    }
    return s
  }
  const path = (id: number) => {
    const out: string[] = []
    for (let g = byId.get(id), hops = 0; g && hops < 100; g = g.parent_id == null ? undefined : byId.get(g.parent_id), hops++) out.unshift(g.name)
    return out
  }
  const rules = (t?.groups ?? []).flatMap(g => (g.ip_ranges ?? []).flatMap(r => {
    const x = parseRange(r)
    return x ? [{ lo: x[0], hi: x[1], id: g.id }] : []
  })).sort((a, b) => a.hi - a.lo - (b.hi - b.lo) || a.id - b.id)
  // ponytail: 장비마다 대역 전체를 훑음 (장비 수천 × 대역 수백까지 충분) — 더 크면 구간 트리로
  const byIP = (ip: string | undefined) => {
    const n = ip ? ip4(ip) : null
    if (n == null) return undefined
    return rules.find(r => r.lo <= n && n <= r.hi)?.id
  }
  return { byId, children, groupOf, subtree, path, byIP }
}
