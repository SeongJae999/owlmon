import { addSNMPDevice, updateSNMPDevice, type SNMPDevice } from './snmp'
import { unitRange, upsertAsset, type Asset } from './asset'
import { saveTopologyLayout, type DeviceKind, type TopologyDevice, type TopologyLayout } from './topology'
import { errorText } from './users'
import type { Rack } from './rack'

export const TEMPLATE_HEADER = ['이름', '종류', 'IP', '상위 장비', '구역', '위치', '랙', 'U', 'U수', '용도', 'SNMP 커뮤니티']
export const TEMPLATE_EXAMPLE = [
  ['인터넷', '인터넷', '', '', '인터넷', '', '', '', '', 'KT 회선', ''],
  ['방화벽', '방화벽', '192.168.0.1', '인터넷', '네트워크', '본관 2층 서버실', 'A랙', '40', '1', '외부 방화벽', 'public'],
  ['코어 스위치', '스위치', '192.168.0.2', '방화벽', '네트워크', '본관 2층 서버실', 'A랙', '38', '1', '', 'public'],
  ['web1', '서버', '192.168.0.11', '코어 스위치', '서버', '본관 2층 서버실', 'A랙', '14', '2', '홈페이지', ''],
]

// 헤더는 띄어쓰기·대소문자를 무시하고 비슷한 이름도 받는다 — 각 기관이 쓰던 장비 대장 열 이름을 그대로 붙여넣는 경우가 많음
const COLUMNS: Record<string, string[]> = {
  name: ['이름', '장비명', '장비이름', '호스트명', 'name', 'hostname'],
  kind: ['종류', '유형', '장비종류', 'kind', 'type'],
  ip: ['ip', 'ip주소', '주소', 'address'],
  parent: ['상위장비', '상위', '위에연결된장비', '연결장비', 'parent', 'uplink'],
  zone: ['구역', 'zone'],
  location: ['위치', '설치위치', 'location'],
  purpose: ['용도', '설명', 'description', 'purpose'],
  community: ['snmp커뮤니티', '커뮤니티', 'community'],
  rack: ['랙', '랙이름', 'rack'],
  unit: ['u', '시작u', '랙위치', 'unit'],
  size: ['u수', '높이u', '크기', 'size'],
}
const norm = (s: string) => s.toLowerCase().replace(/\s+/g, '')

const KINDS: [DeviceKind, string[]][] = [
  ['server', ['서버', 'server', 'vm', '가상서버']],
  ['network', ['스위치', '네트워크', '라우터', 'l2', 'l3', 'l4', 'switch', 'network', 'router', 'ap', '무선ap']],
  ['firewall', ['방화벽', '보안', 'firewall', 'utm', 'ips']],
  ['internet', ['인터넷', '회선', 'internet', 'isp']],
]
export const KIND_NAME: Record<DeviceKind, string> = { server: '서버', network: '스위치', firewall: '방화벽', internet: '인터넷' }
const KIND_ORDER: DeviceKind[] = ['internet', 'firewall', 'network', 'server']
const DEFAULT_ZONE: Record<DeviceKind, string> = { internet: '인터넷', firewall: '네트워크', network: '네트워크', server: '서버' }

// 앞자리 0(192.168.01.5)은 받지 않음 — 서버는 이를 IP가 아닌 호스트 이름으로 보고, 기존 192.168.1.5와 중복으로도 안 잡힘
const IPV4 = /^(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}$/
// IPv6는 브라우저 URL 파서에 맡김 — 포트 붙은 IPv4(1.2.3.4:161)·MAC 주소를 IP로 받지 않게
const isIPv6 = (s: string) => { try { return s.includes(':') && new URL(`http://[${s}]/`).hostname !== '' } catch { return false } }
const isIP = (s: string) => IPV4.test(s) || isIPv6(s)
// 상위 장비 칸에 이렇게 적으면 기존 연결을 끊고 맨 위로 — 빈 칸은 기존 연결을 그대로 둠
const TOP_WORDS = ['맨위', '없음', '-', 'none', 'top']

export interface PlanRow {
  line: number
  name: string
  kind?: DeviceKind
  ip: string
  parent: string
  zone: string
  location: string
  purpose: string
  community: string
  rack: string // 비우면 기존 랙 위치 유지, '없음'·'-'면 랙에서 뺌
  unit: number
  size: number
  snmp: 'add' | 'exists' | 'community' | 'none'
  snmpDevice?: SNMPDevice // 이미 감시 중인 장비 — 커뮤니티만 바꿀 때 씀
  asset: boolean
  errors: string[]
  warnings: string[]
}

export interface Plan {
  rows: PlanRow[]
  layout: TopologyLayout
  assetOrder: number[] // 장비 정보 저장 순서(줄 번호) — 다른 장비가 비우는 칸에 들어가는 줄은 그 장비 다음에
  headerError?: string
}

export interface PlanContext {
  layout: TopologyLayout // 저장된 구성도, 없으면 자동 배치
  snmp: SNMPDevice[]
  assets: Asset[]
  servers: string[] // 에이전트가 연결된 서버
  racks: Rack[]
}

export function planImport(table: string[][], ctx: PlanContext): Plan {
  const empty = { rows: [], layout: ctx.layout, assetOrder: [] }
  if (table.length < 2) return { ...empty, headerError: '첫 줄에 열 이름, 둘째 줄부터 장비를 적어 주세요' }
  // 맨 앞 빈 줄(붙여넣기 전에 Enter)은 건너뛰고 첫 내용 줄을 열 이름으로 — 줄 번호는 원래 행 번호 유지
  const headerAt = table.findIndex(r => r.some(c => c !== ''))
  if (headerAt < 0) return { ...empty, headerError: '첫 줄에 열 이름, 둘째 줄부터 장비를 적어 주세요' }
  const header = table[headerAt].map(norm)
  const col: Record<string, number> = {}
  // 별칭 순서가 우선순위 — '주소'(설치 주소)가 'IP' 열보다 앞에 있어도 'IP' 열을 고름
  for (const [key, names] of Object.entries(COLUMNS)) col[key] = names.map(n => header.indexOf(n)).find(i => i >= 0) ?? -1
  if (col.name < 0 || col.kind < 0) return { ...empty, headerError: '"이름"과 "종류" 열이 필요합니다. 양식을 받아 첫 줄을 그대로 써 주세요' }

  const cell = (r: string[], key: string) => (col[key] >= 0 ? r[col[key]] ?? '' : '').trim()
  const placed = new Map(ctx.layout.devices.map(d => [d.name, d]))
  // 빈 줄은 건너뛰되 줄 번호는 엑셀 행 번호 그대로
  const rows: PlanRow[] = table.slice(headerAt + 1).map((r, i) => ({ r, line: headerAt + i + 2 })).filter(({ r }) => r.some(c => c !== '')).map(({ r, line }) => {
    const kindText = norm(cell(r, 'kind'))
    const kind = KINDS.find(([, names]) => names.includes(kindText))?.[0]
    const name = cell(r, 'name')
    const prev = placed.get(name)
    const parentText = cell(r, 'parent')
    const top = TOP_WORDS.includes(norm(parentText))
    const row: PlanRow = {
      line, name, kind, ip: cell(r, 'ip'),
      // 이미 구성도에 있는 장비는 비운 칸(또는 열이 없는 파일)이 손으로 그린 연결·구역을 지우지 않게 기존 값을 이어받는다
      parent: top ? '' : parentText || prev?.parent || '',
      zone: cell(r, 'zone') || prev?.zone || (kind ? DEFAULT_ZONE[kind] : ''),
      location: cell(r, 'location'), purpose: cell(r, 'purpose'),
      community: cell(r, 'community'), rack: '', unit: 0, size: 1, snmp: 'none', asset: false, errors: [], warnings: [],
    }
    // 랙도 비운 칸은 기존 값 유지 — 위치만 고치려고 올린 대장이 랙 배치를 지우면 안 됨
    const prevAsset = ctx.assets.find(a => a.host_name === name)
    const rackText = cell(r, 'rack'), unitText = cell(r, 'unit'), sizeText = cell(r, 'size')
    const toInt = (t: string) => /^\d+$/.test(t) ? Number(t) : NaN
    if (TOP_WORDS.includes(norm(rackText))) {
      row.rack = ''
    } else if (rackText) {
      row.rack = rackText
      row.unit = toInt(unitText)
      // U수를 비우면 장비 높이는 그대로 — 다른 랙으로 옮겨도 장비 크기는 같음
      row.size = sizeText ? toInt(sizeText) : prevAsset?.rack ? prevAsset.rack_size || 1 : 1
      if (!unitText) row.errors.push('랙을 적었으면 시작 U도 적어 주세요 (맨 아래 칸이 1)')
      else if (!(row.unit >= 1)) row.errors.push('U는 1 이상의 숫자로 적어 주세요')
      if (!(row.size >= 1)) row.errors.push('U수는 1 이상의 숫자로 적어 주세요')
    } else {
      row.rack = prevAsset?.rack ?? ''
      row.unit = prevAsset?.rack_unit ?? 0
      row.size = prevAsset?.rack_size || 1
      if (unitText || sizeText) row.errors.push('U·U수를 적었으면 랙 이름도 적어 주세요')
    }
    if (prev && (prev.parent ?? '') !== row.parent) row.warnings.push(`구성도 연결이 바뀝니다: ${prev.parent || '맨 위'} → ${row.parent || '맨 위'}`)
    if (prev && prev.zone !== row.zone) row.warnings.push(`구역이 바뀝니다: ${prev.zone} → ${row.zone}`)
    return row
  })
  if (rows.length === 0) return { ...empty, headerError: '둘째 줄부터 장비를 적어 주세요' }

  const fileNames = new Map<string, number>()
  for (const r of rows) fileNames.set(r.name, (fileNames.get(r.name) ?? 0) + 1)
  const isSNMPKind = (r: PlanRow) => r.kind === 'network' || r.kind === 'firewall'
  // 파일 안에서 같은 IP가 두 줄이면 둘째 줄 등록이 서버에서 거절돼, 감시 없이 구성도에만 남음
  const fileIPs = new Map<string, number>()
  for (const r of rows) if (isSNMPKind(r) && r.ip) fileIPs.set(r.ip, (fileIPs.get(r.ip) ?? 0) + 1)
  const known = new Set([...ctx.layout.devices.map(d => d.name), ...ctx.servers, ...ctx.snmp.map(d => d.Name)])

  for (const r of rows) {
    if (!r.name) r.errors.push('이름이 비었습니다')
    else if (fileNames.get(r.name)! > 1) r.errors.push('같은 이름이 파일에 두 번 이상 있습니다')
    if (!r.kind) r.errors.push(`종류를 알 수 없습니다 — 서버·스위치·방화벽·인터넷 중 하나로 적어 주세요`)
    if (r.ip && !isIP(r.ip)) r.errors.push('IP 형식이 아닙니다')
    if (r.parent === r.name && r.name) r.errors.push('자기 자신을 상위 장비로 적었습니다')
    else if (r.parent && !fileNames.has(r.parent) && !known.has(r.parent)) r.errors.push(`상위 장비 "${r.parent}"를 찾을 수 없습니다`)

    const sameName = ctx.snmp.find(d => d.Name === r.name)
    if (isSNMPKind(r) && r.ip) {
      const sameIP = ctx.snmp.find(d => d.IP === r.ip)
      if (fileIPs.get(r.ip)! > 1) r.errors.push('같은 IP가 파일에 두 번 이상 있습니다')
      else if (sameIP && sameIP.Name !== r.name) r.errors.push(`이 IP는 이미 "${sameIP.Name}"(으)로 등록되어 있습니다`)
      else if (sameName && sameName.IP !== r.ip) r.errors.push(`같은 이름이 다른 IP(${sameName.IP})로 등록되어 있습니다`)
      else r.snmp = sameIP ? 'exists' : 'add'
      r.snmpDevice = sameIP
    } else if (isSNMPKind(r) && sameName) {
      r.snmp = 'exists'
      r.snmpDevice = sameName
    } else if (isSNMPKind(r)) {
      r.warnings.push('IP가 없어 상태는 감시하지 않고 구성도에만 그립니다')
    }
    // 처음에 커뮤니티를 비워 public으로 등록한 장비를, 대장에 실제 값을 채워 다시 올리면 고쳐지게 — 서버가 기존 값을 안 알려 주므로 적혀 있으면 늘 갱신
    if (r.snmp === 'exists' && r.community) r.snmp = 'community'
    if (r.kind === 'server' && r.name && !ctx.servers.includes(r.name)) {
      r.warnings.push('아직 연결되지 않은 서버 — 에이전트를 설치하면 같은 이름으로 이어집니다')
    }
    if (r.community && r.snmp === 'none') r.warnings.push('SNMP 등록 대상이 아니어서 커뮤니티 값은 쓰지 않습니다')

    // 비운 칸은 기존 값을 지우지 않는다 — 위치만 적은 줄이 기존 용도·IP를 날리면 안 됨
    const prev = ctx.assets.find(a => a.host_name === r.name)
    const rackChanged = r.rack !== (prev?.rack ?? '') || (!!r.rack && (r.unit !== prev?.rack_unit || r.size !== prev?.rack_size))
    r.asset = !!r.name && ((!!r.location && r.location !== prev?.location) || (!!r.purpose && r.purpose !== prev?.description) || (!!r.ip && r.ip !== prev?.ip) || rackChanged)
  }

  // 기존 구성도에 파일 내용을 덮어씀 — 파일에 없는 장비와 이중화 짝은 그대로 둔다
  const devices = new Map<string, TopologyDevice>(ctx.layout.devices.map(d => [d.name, d]))
  for (const r of rows) {
    if (!r.name || !r.kind) continue
    const prev = devices.get(r.name)
    devices.set(r.name, { name: r.name, kind: r.kind, zone: r.zone, ...(r.parent ? { parent: r.parent } : {}), ...(prev?.pair ? { pair: prev.pair } : {}) })
  }
  // 구역은 위에서 아래로 그려짐 — 새 구역을 맨 끝에 붙이면 방화벽 구역이 서버 아래에 그려지므로, 담긴 장비 종류(인터넷→방화벽→스위치→서버) 순서에 맞는 자리에 끼운다
  const rank = (z: string) => Math.min(...[...devices.values()].filter(d => d.zone === z).map(d => KIND_ORDER.indexOf(d.kind)), KIND_ORDER.length)
  const zones = [...ctx.layout.zones]
  for (const r of rows) {
    if (!r.zone || zones.includes(r.zone)) continue
    const at = zones.findIndex(z => rank(z) > rank(r.zone))
    zones.splice(at < 0 ? zones.length : at, 0, r.zone)
  }
  const layout = { zones, devices: [...devices.values()] }

  // 상위 장비를 따라 올라가다 자기 자신을 다시 만나면 순환 — 저장 단계에서 서버가 거부하기 전에 해당 줄에 표시
  const parentOf = new Map(layout.devices.map(d => [d.name, d.parent]))
  for (const r of rows) {
    const seen = new Set<string>()
    for (let n: string | undefined = r.name; n; n = parentOf.get(n)) {
      if (seen.has(n)) { r.errors.push('상위 장비 연결이 돌고 돕니다 (A→B→A)'); break }
      seen.add(n)
    }
    // 파일에 없는 기존 장비를 상위로 적었는데 그 장비가 구성도에 없으면, 저장 시 "상위 장비 없음"으로 거부됨
    if (r.parent && !parentOf.has(r.parent) && !r.errors.some(e => e.includes('상위 장비'))) {
      r.errors.push(`상위 장비 "${r.parent}"가 구성도에 없습니다 — 그 장비도 파일에 한 줄 넣어 주세요`)
    }
  }
  // 랙 높이·겹침 — 파일에 없는 기존 장비와, 파일 안의 장비끼리 모두 본다 (서버도 같은 검사를 하지만 미리 줄마다 알려 줌)
  const heightOf = (rack: string) => ctx.racks.find(k => k.name === rack)?.height ?? 42
  const inFile = new Set(rows.map(r => r.name))
  const occupants = [
    ...ctx.assets.filter(a => a.rack && a.rack_unit > 0 && !inFile.has(a.host_name)).map(a => ({ name: a.host_name, rack: a.rack, unit: a.rack_unit, size: a.rack_size || 1, row: undefined as PlanRow | undefined })),
    ...rows.filter(r => r.rack && r.unit >= 1 && r.size >= 1).map(r => ({ name: r.name, rack: r.rack, unit: r.unit, size: r.size, row: r as PlanRow | undefined })),
  ]
  for (const o of occupants) {
    if (!o.row) continue
    const h = heightOf(o.rack)
    if (o.unit + o.size - 1 > h) o.row.errors.push(`${o.rack} 높이(${h}U)를 넘습니다 — ${unitRange(o.unit, o.size)}`)
    const hit = occupants.find(x => x !== o && x.rack === o.rack && x.unit <= o.unit + o.size - 1 && x.unit + x.size - 1 >= o.unit)
    if (hit) o.row.errors.push(`${o.rack} ${unitRange(hit.unit, hit.size)}의 ${hit.name}와 칸이 겹칩니다`)
  }

  // 서버는 한 줄씩 저장하며 그 순간의 칸을 검사함 — 파일 안에서 A가 비우는 칸으로 B가 들어가면 A를 먼저 저장해야 거절되지 않음
  const prevOf = (r: PlanRow) => ctx.assets.find(a => a.host_name === r.name && a.rack && a.rack_unit > 0)
  const savers = rows.filter(r => r.asset && !r.errors.length)
  const leaves = (m: PlanRow) => { const p = prevOf(m); return !!p && (p.rack !== m.rack || p.rack_unit !== m.unit || (p.rack_size || 1) !== m.size) }
  const waitsFor = new Map(savers.map(r => [r, savers.filter(m => {
    const p = m !== r && leaves(m) ? prevOf(m) : undefined
    return !!p && !!r.rack && p.rack === r.rack && p.rack_unit <= r.unit + r.size - 1 && p.rack_unit + (p.rack_size || 1) - 1 >= r.unit
  })]))
  const assetOrder: number[] = []
  for (let left = [...savers], progress = true; left.length && progress;) {
    progress = false
    for (const r of [...left]) {
      if (waitsFor.get(r)!.every(m => assetOrder.includes(m.line))) { assetOrder.push(r.line); left.splice(left.indexOf(r), 1); progress = true }
    }
    if (!progress) for (const r of left) r.errors.push('다른 장비와 칸을 맞바꾸는 줄은 한 번에 저장할 수 없습니다 — 한 장비를 먼저 빈 칸으로 옮겨 올린 뒤 다시 올려 주세요')
  }

  // 오류 줄은 등록하지 않으니 할 일·건수에서도 뺀다
  for (const r of rows) if (r.errors.length) { r.snmp = 'none'; r.asset = false }
  return { rows, layout, assetOrder: assetOrder.filter(l => rows.some(r => r.line === l && r.asset)) }
}

export interface ApplyResult {
  line: number
  name: string
  ok: boolean
  messages: string[]
}

export async function applyImport(plan: Plan, assets: Asset[]): Promise<{ results: ApplyResult[]; layoutError?: string }> {
  const results: ApplyResult[] = []
  // 한 줄이 실패해도 나머지는 계속 — 어느 줄이 왜 안 됐는지 끝에 한꺼번에 보여 줌
  for (const r of plan.rows) {
    const res: ApplyResult = { line: r.line, name: r.name, ok: true, messages: [] }
    if (r.snmp === 'add') {
      try {
        await addSNMPDevice({ Name: r.name, IP: r.ip, Port: 161, Community: r.community || 'public' })
        res.messages.push('감시 등록')
      } catch (e) {
        res.ok = false
        res.messages.push(`감시 등록 실패: ${errorText(e, '서버 오류')}`)
      }
    }
    if (r.snmp === 'community' && r.snmpDevice) {
      const d = r.snmpDevice
      try {
        await updateSNMPDevice(d.ID, { Name: d.Name, IP: d.IP, Port: d.Port, Community: r.community })
        res.messages.push('커뮤니티 변경')
      } catch (e) {
        res.ok = false
        res.messages.push(`커뮤니티 변경 실패: ${errorText(e, '서버 오류')}`)
      }
    }
    results.push(res)
  }
  for (const line of plan.assetOrder) {
    const r = plan.rows.find(x => x.line === line)!
    const res = results.find(x => x.line === line)!
    const prev = assets.find(a => a.host_name === r.name)
    try {
      await upsertAsset({
        host_name: r.name,
        ip: r.ip || prev?.ip || '',
        location: r.location || prev?.location || '',
        description: r.purpose || prev?.description || '',
        purchase_date: prev?.purchase_date ?? '',
        warranty_expires: prev?.warranty_expires ?? '',
        notes: prev?.notes ?? '',
        rack: r.rack,
        rack_unit: r.rack ? r.unit : 0,
        rack_size: r.rack ? r.size : 1,
      })
      res.messages.push('장비 정보 저장')
    } catch (e) {
      res.ok = false
      res.messages.push(`장비 정보 저장 실패: ${errorText(e, '서버 오류')}`)
    }
  }
  try {
    await saveTopologyLayout(plan.layout)
  } catch (e) {
    return { results, layoutError: errorText(e, '구성도를 저장하지 못했습니다') }
  }
  return { results }
}
