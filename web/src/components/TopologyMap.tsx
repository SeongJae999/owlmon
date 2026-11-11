import { useNavigate } from 'react-router-dom'
import { Globe, Network, Server, Shield } from 'lucide-react'
import { layoutTopology, NODE_H, NODE_W, type DeviceKind, type TopologyLayout } from '../api/topology'
import { cn } from '../utils/cn'

export type NodeState = 'crit' | 'warn' | 'maint' | 'ok' | 'wait'

interface Props {
  layout: TopologyLayout
  lookup: (name: string) => { state: NodeState; detail: string; to: string; down?: boolean } | undefined
}

const PAD = 24
const NAME_SIZE = 21

// 한글은 글자 크기의 약 0.92배, 영문·숫자는 약 0.58배 폭으로 어림해 상자 안에 들어가게 자른다
function fit(name: string, max = NODE_W - 52) {
  let w = 0
  for (let i = 0; i < name.length; i++) {
    w += (/[\u3131-\uD79D]/.test(name[i]) ? 0.92 : 0.58) * NAME_SIZE
    if (w > max) return name.slice(0, Math.max(1, i - 1)) + '…'
  }
  return name
}
const ICON = { internet: Globe, firewall: Shield, network: Network, server: Server } satisfies Record<DeviceKind, unknown>
const BOX: Record<NodeState, string> = {
  crit: 'fill-rose-950 stroke-rose-500 [stroke-width:2.5]',
  warn: 'fill-amber-950 stroke-amber-500 [stroke-width:2]',
  maint: 'fill-base-900 stroke-base-700 opacity-60',
  ok: 'fill-base-900 stroke-emerald-600',
  wait: 'fill-base-900 stroke-base-800',
}
const LABEL: Record<NodeState, string> = { crit: '위험', warn: '주의', maint: '점검 중', ok: '정상', wait: '확인 중' }
const LABEL_FILL: Record<NodeState, string> = {
  crit: 'fill-rose-300', warn: 'fill-amber-300', maint: 'fill-purple-300', ok: 'fill-emerald-400', wait: 'fill-base-500',
}
const ICON_COLOR: Record<NodeState, string> = {
  crit: 'text-rose-300', warn: 'text-amber-300', maint: 'text-base-500', ok: 'text-emerald-300', wait: 'text-base-600',
}

export default function TopologyMap({ layout, lookup }: Props) {
  const navigate = useNavigate()
  const scene = layoutTopology(layout)
  const at = new Map(scene.nodes.map(n => [n.device.name, n]))
  const isDown = (name: string) => !!lookup(name)?.down
  const stateOf = (kind: DeviceKind, name: string): NodeState => kind === 'internet' ? 'ok' : lookup(name)?.state ?? 'wait'
  // 위쪽 장비가 끊겨 함께 끊긴 경우: 원인은 위에 있다고 알려 줌
  const causedAbove = (name: string) => {
    for (let p = at.get(name)?.device.parent, hops = 0; p && hops < 50; p = at.get(p)?.device.parent, hops++) {
      if (isDown(p)) return true
    }
    return false
  }

  return (
    <svg
      viewBox={`${-PAD} ${-PAD} ${scene.width + PAD * 2} ${scene.height + PAD * 2}`}
      preserveAspectRatio="xMidYMin meet" className="w-full h-full" role="img" aria-label="네트워크 구성도"
    >
      {scene.zones.map(z => (
        <g key={z.name}>
          <line x1={-PAD / 2} x2={scene.width + PAD / 2} y1={z.y} y2={z.y} className="stroke-base-800" />
          <text x={-PAD / 2 + 16} y={z.y + z.h / 2 + 7} fontSize={19} fontWeight={700} className="fill-base-500">{z.name}</text>
        </g>
      ))}

      {scene.nodes.map(n => {
        const p = n.device.parent ? at.get(n.device.parent) : undefined
        if (!p) return null
        const down = isDown(n.device.name) || isDown(p.device.name)
        const top = n.y - NODE_H / 2, bottom = p.y + NODE_H / 2, bus = top - 20
        const d = n.trunkX !== undefined
          ? `M${p.x},${bottom} V${bottom + 20} H${n.trunkX} V${bus} H${n.x} V${top}`
          : `M${p.x},${bottom} V${bus} H${n.x} V${top}`
        // 형제와 같이 쓰는 선까지 빨갛게 칠하면 다른 장비도 끊긴 것처럼 보여서 이 장비로 내려가는 마지막 구간만 표시
        const only = scene.nodes.filter(m => m.device.parent === p.device.name).length === 1
        const drop = only ? d : `M${n.x},${bus} V${top}`
        return (
          <g key={`l-${n.device.name}`}>
            <path d={d} fill="none" strokeWidth={1.5} className="stroke-base-700" />
            {down && <path d={drop} fill="none" strokeWidth={3} strokeDasharray="7 5" className="stroke-rose-500" />}
          </g>
        )
      })}

      {scene.nodes.map(n => {
        const mate = n.device.pair ? at.get(n.device.pair) : undefined
        if (!mate || mate.y !== n.y || n.x > mate.x) return null
        const x1 = n.x + NODE_W / 2, x2 = mate.x - NODE_W / 2
        return (
          <g key={`p-${n.device.name}`} className="stroke-base-500" strokeWidth={1.5}>
            <line x1={x1} x2={x2} y1={n.y - 3} y2={n.y - 3} />
            <line x1={x1} x2={x2} y1={n.y + 3} y2={n.y + 3} />
          </g>
        )
      })}

      {scene.nodes.map(n => {
        const { name, kind } = n.device
        const info = kind === 'internet' ? undefined : lookup(name)
        const state = stateOf(kind, name)
        const caused = isDown(name) && causedAbove(name)
        const go = info?.to ? () => navigate(info.to) : undefined
        const Icon = ICON[kind]
        // 정상은 상태 글자를 빼서 조용하게 두고, 문제 있는 장비만 눈에 띄게
        const quiet = kind === 'internet' || state === 'ok'
        return (
          <g
            key={name}
            transform={`translate(${n.x - NODE_W / 2},${n.y - NODE_H / 2})`}
            onClick={go}
            onKeyDown={go ? e => { if (e.key === 'Enter') go() } : undefined}
            role={go ? 'link' : undefined}
            tabIndex={go ? 0 : undefined}
            className={go ? 'cursor-pointer outline-none [&:focus-visible>rect]:stroke-brand-300 [&:hover>rect]:stroke-base-500' : undefined}
          >
            <title>{info?.detail ? `${name}: ${info.detail}` : name}</title>
            <rect
              width={NODE_W} height={NODE_H} rx={3}
              strokeDasharray={kind === 'internet' ? '5 4' : undefined}
              className={kind === 'internet' ? 'fill-base-950 stroke-base-600' : BOX[state]}
            />
            <Icon x={12} y={NODE_H / 2 - 12} width={24} height={24} className={ICON_COLOR[state]} />
            <text
              x={44} y={quiet ? NODE_H / 2 + 8 : 27} fontSize={NAME_SIZE} fontWeight={600}
              className={state === 'ok' || kind === 'internet' ? 'fill-base-200' : 'fill-base-50'}
            >
              {fit(name)}
            </text>
            {!quiet && (
              <text
                x={44} y={50} fontSize={15} fontWeight={700}
                className={cn(LABEL_FILL[state], state === 'crit' && !caused && 'motion-safe:animate-pulse')}
              >
                {caused ? '상위 장비 영향' : LABEL[state]}
              </text>
            )}
          </g>
        )
      })}
    </svg>
  )
}
