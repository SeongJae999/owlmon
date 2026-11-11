import axios from 'axios'

export type DeviceKind = 'internet' | 'firewall' | 'network' | 'server'

export interface TopologyDevice {
  name: string
  kind: DeviceKind
  zone: string
  parent?: string
  pair?: string
}

export interface TopologyLayout {
  zones: string[]
  devices: TopologyDevice[]
}

export async function getTopologyLayout(): Promise<TopologyLayout> {
  const res = await axios.get('/api/topology')
  return res.data
}

export async function saveTopologyLayout(layout: TopologyLayout): Promise<void> {
  await axios.put('/api/topology', layout)
}

// ponytail: 배선 정보가 없을 때의 기본 그림. 스위치가 1대일 때만 서버를 그 아래로 연결
export function defaultLayout(servers: string[], networks: string[]): TopologyLayout {
  const parent = networks.length === 1 ? networks[0] : '인터넷'
  return {
    zones: ['인터넷', '네트워크', '서버'],
    devices: [
      { name: '인터넷', kind: 'internet', zone: '인터넷' },
      ...networks.map(name => ({ name, kind: 'network' as const, zone: '네트워크', parent: '인터넷' })),
      ...servers.map(name => ({ name, kind: 'server' as const, zone: '서버', parent })),
    ],
  }
}

export const NODE_W = 176
export const NODE_H = 62
const SLOT = 192
const ROW_H = 84
const ZONE_PAD = 10
const ZONE_GAP = 10
// 구역 이름은 위가 아니라 왼쪽 여백에 써서 세로 공간을 아낀다
export const ZONE_LABEL_W = 120
// 자식이 이보다 많고 전부 말단이면 옆으로 늘어놓지 않고 격자로 접어 화면 폭을 아낀다
const GRID_COLS = 4

export interface PlacedNode {
  device: TopologyDevice
  x: number
  y: number
  // 격자 2번째 줄 이하는 위 줄 상자를 피해 왼쪽 세로선(trunk)으로 연결
  trunkX?: number
}

export interface Scene {
  nodes: PlacedNode[]
  zones: { name: string; y: number; h: number }[]
  width: number
  height: number
}

export function layoutTopology(layout: TopologyLayout): Scene {
  const byName = new Map(layout.devices.map(d => [d.name, d]))
  const children = new Map<string, TopologyDevice[]>()
  const roots: TopologyDevice[] = []
  for (const d of layout.devices) {
    if (d.parent && d.parent !== d.name && byName.has(d.parent)) {
      children.set(d.parent, [...(children.get(d.parent) ?? []), d])
    } else roots.push(d)
  }
  // 이중화 짝은 나란히 놓이도록 짝 바로 뒤로 정렬
  for (const [k, list] of children) {
    const ordered: TopologyDevice[] = []
    for (const d of list) {
      if (ordered.includes(d)) continue
      ordered.push(d)
      const mate = list.find(m => m !== d && (m.pair === d.name || d.pair === m.name))
      if (mate && !ordered.includes(mate)) ordered.push(mate)
    }
    children.set(k, ordered)
  }

  const zones = [...layout.zones]
  for (const d of layout.devices) if (!zones.includes(d.zone)) zones.push(d.zone)

  const isMate = (a: TopologyDevice, b: TopologyDevice) => a.zone === b.zone && (a.pair === b.name || b.pair === a.name)
  // 하위 장비가 있는 장비의 짝이 말단이면, 그 줄엔 짝 둘뿐이라 따로 칸을 잡지 않고 옆에 붙인다
  // 짝이 사슬처럼 이어진 잘못된 데이터(a-b, b-c)는 앞의 한 쌍만 붙인다
  const pairedWithPrev = (list: TopologyDevice[], i: number) =>
    i > 0 && isMate(list[i - 1], list[i]) && !(i > 1 && isMate(list[i - 2], list[i - 1]))
  const absorbed = (list: TopologyDevice[], i: number, path: Set<string>) =>
    pairedWithPrev(list, i) && !children.get(list[i].name)?.length && width(list[i - 1].name, path) >= 2
  const isGrid = (name: string) => {
    const list = children.get(name) ?? []
    return list.length > GRID_COLS && list.every(c => !children.get(c.name)?.length)
  }
  const widthMemo = new Map<string, number>()
  const width = (name: string, path = new Set<string>()): number => {
    if (widthMemo.has(name)) return widthMemo.get(name)!
    const list = children.get(name) ?? []
    let w = 1
    if (isGrid(name)) w = GRID_COLS
    else if (list.length && !path.has(name)) {
      const next = new Set(path).add(name)
      w = list.reduce((s, c, i) => s + (absorbed(list, i, next) ? 0 : width(c.name, next)), 0)
    }
    widthMemo.set(name, w)
    return w
  }

  const placed = new Map<string, { device: TopologyDevice; slot: number; row: number; gridRow: number; trunkSlot?: number }>()
  const place = (d: TopologyDevice, start: number, parentRow: number | null, parentZone: string | null) => {
    if (placed.has(d.name)) return
    const row = parentRow !== null && parentZone === d.zone ? parentRow + 1 : 0
    placed.set(d.name, { device: d, slot: start + width(d.name) / 2, row, gridRow: 0 })
    const list = children.get(d.name) ?? []
    if (isGrid(d.name)) {
      list.forEach((c, i) => {
        const gridRow = Math.floor(i / GRID_COLS)
        const base = d.zone === c.zone ? row + 1 : 0
        placed.set(c.name, { device: c, slot: start + (i % GRID_COLS) + 0.5, row: base + gridRow, gridRow, trunkSlot: start })
      })
      return
    }
    let cursor = start
    list.forEach((c, i) => {
      const prev = list[i - 1]
      const own = absorbed(list, i, new Set())
      const start = own ? cursor - width(prev.name) : cursor
      place(c, start, row, d.zone)
      // 이중화 짝은 둘이 차지한 폭의 가운데에 붙여서 놓는다
      if (pairedWithPrev(list, i)) {
        const a = placed.get(prev.name)!, b = placed.get(c.name)!
        const mid = own ? start + width(prev.name) / 2 : (start - width(prev.name) + start + width(c.name)) / 2
        if (a.row === b.row) { a.slot = mid - 0.5; b.slot = mid + 0.5 }
      }
      if (!own) cursor += width(c.name)
    })
  }
  let cursor = 0
  const visit = (d: TopologyDevice) => {
    place(d, cursor, null, null)
    cursor += width(d.name)
  }
  roots.forEach(visit)
  // 순환으로 어느 뿌리에서도 닿지 않은 장비도 빠짐없이 그린다
  layout.devices.filter(d => !placed.has(d.name)).forEach(visit)

  const zoneRows = new Map<string, number>()
  for (const p of placed.values()) zoneRows.set(p.device.zone, Math.max(zoneRows.get(p.device.zone) ?? 0, p.row + 1))
  const zoneBands: Scene['zones'] = []
  let y = 0
  for (const name of zones) {
    const rows = zoneRows.get(name)
    if (!rows) continue
    const h = ZONE_PAD * 2 + rows * ROW_H
    zoneBands.push({ name, y, h })
    y += h + ZONE_GAP
  }
  const bandY = new Map(zoneBands.map(z => [z.name, z.y]))

  const nodes: PlacedNode[] = [...placed.values()].map(p => ({
    device: p.device,
    x: ZONE_LABEL_W + p.slot * SLOT,
    y: bandY.get(p.device.zone)! + ZONE_PAD + p.row * ROW_H + ROW_H / 2,
    trunkX: p.gridRow > 0 && p.trunkSlot !== undefined ? ZONE_LABEL_W + p.trunkSlot * SLOT + 4 : undefined,
  }))

  return { nodes, zones: zoneBands, width: ZONE_LABEL_W + Math.max(cursor, 1) * SLOT, height: Math.max(y - ZONE_GAP, ROW_H) }
}
