import axios from 'axios'

export type MemberKind = 'server' | 'snmp' | 'synthetic' | 'ssl' | 'dpm'

export interface DeviceGroup {
  id: number
  parent_id: number | null
  name: string
  sort: number
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

export async function createGroup(g: { name: string; parent_id: number | null }): Promise<DeviceGroup> {
  const res = await axios.post<DeviceGroup>('/api/groups', { ...g, sort: 0 })
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
  return { byId, children, groupOf, subtree, path }
}
