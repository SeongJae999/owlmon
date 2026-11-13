import { useNavigate } from 'react-router-dom'
import { Globe, Network, Server, Shield } from 'lucide-react'
import { layoutTopology, NODE_H, NODE_W, type DeviceKind, type TopologyLayout } from '../api/topology'
import { cn } from '../utils/cn'

export type NodeState = 'crit' | 'warn' | 'maint' | 'ok' | 'wait'

interface Props {
  layout: TopologyLayout
  lookup: (name: string) => { state: NodeState; detail: string; to: string; down?: boolean; acked?: unknown; causedBy?: string } | undefined
  // 편집 화면: 누르면 이동하지 않고 장비를 고름
  onPick?: (name: string) => void
  picked?: string | null
  blocked?: (name: string) => boolean
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

export default function TopologyMap({ layout, lookup, onPick, picked, blocked }: Props) {
  const navigate = useNavigate()
  const scene = layoutTopology(layout)
  const at = new Map(scene.nodes.map(n => [n.device.name, n]))
  const isDown = (name: string) => !!lookup(name)?.down
  // 선이 흐르는 건 아직 아무도 확인하지 않은 실제 장애뿐 — 확인했거나 점검 중이면 멈춤
  const live = (name: string) => isDown(name) && lookup(name)?.state !== 'maint' && !lookup(name)?.acked
  const stateOf = (kind: DeviceKind, name: string): NodeState => kind === 'internet' ? 'ok' : lookup(name)?.state ?? 'wait'

  return (
    <svg
      viewBox={`${-PAD} ${-PAD} ${scene.width + PAD * 2} ${scene.height + PAD * 2}`}
      // 실제 크기보다 키우지 않음(확대되면 상자·글씨가 뭉툭해짐), 0.6배보다 줄이지 않음(글씨가 안 읽힘) — 넘치면 바깥이 스크롤
      style={{
        maxWidth: scene.width + PAD * 2, maxHeight: scene.height + PAD * 2,
        minWidth: (scene.width + PAD * 2) * 0.6, minHeight: (scene.height + PAD * 2) * 0.6,
      }}
      preserveAspectRatio="xMidYMin meet" className="block mx-auto w-full h-full" role="group" aria-label="네트워크 구성도"
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
            {down && (
              <path d={drop} fill="none" strokeWidth={3} strokeDasharray="7 5"
                className={cn('stroke-rose-500', live(isDown(p.device.name) && lookup(p.device.name)?.state !== 'maint' ? p.device.name : n.device.name) && 'motion-safe:animate-flow')} />
            )}
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
        // 원인 판정은 useFleet 한 곳에서만 — 여기서 따로 따지면 목록과 구성도가 서로 다르게 보임 (점검 중 장비 등)
        const caused = !!info?.causedBy
        const off = !!onPick && !!blocked?.(name)
        const go = onPick ? (off ? undefined : () => onPick(name)) : info?.to ? () => navigate(info.to) : undefined
        const Icon = ICON[kind]
        // 정상은 상태 글자를 빼서 조용하게 두고, 문제 있는 장비만 눈에 띄게
        const quiet = kind === 'internet' || state === 'ok'
        // 담당자가 확인한 문제는 계속 깜빡이며 시선을 끌지 않게 중립색으로
        const handled = !!info?.acked && (state === 'crit' || state === 'warn')
        return (
          <g
            key={name}
            transform={`translate(${n.x - NODE_W / 2},${n.y - NODE_H / 2})`}
            onClick={go}
            onKeyDown={go ? e => { if (e.key === 'Enter' || (onPick && e.key === ' ')) { e.preventDefault(); go() } } : undefined}
            role={go ? (onPick ? 'button' : 'link') : undefined}
            tabIndex={go ? 0 : undefined}
            aria-label={onPick ? (picked === name ? `${name} 선택 취소` : picked ? `${picked}을(를) ${name} 아래로 연결` : `${name} 선택`) : undefined}
            aria-pressed={onPick ? picked === name : undefined}
            className={cn(go && 'cursor-pointer outline-none [&:focus-visible>rect]:stroke-brand-300 [&:hover>rect]:stroke-base-500', off && 'opacity-30')}
          >
            <title>{info?.detail ? `${name}: ${info.detail}` : name}</title>
            <rect
              width={NODE_W} height={NODE_H} rx={3}
              // 아직 아무도 확인하지 않은 원인 장비만 깜빡임 — 함께 끊긴 하위 장비까지 깜빡이면 원인을 못 찾음
              strokeDasharray={kind === 'internet' ? '5 4' : undefined}
              className={cn(kind === 'internet' ? 'fill-base-950 stroke-base-600' : handled ? 'fill-base-900 stroke-base-500 [stroke-width:2]' : BOX[state],
                state === 'crit' && !caused && !handled && 'motion-safe:animate-blink',
                picked === name && '!stroke-brand-500 !fill-brand-500/15 [stroke-width:3]')}
            />
            <Icon x={12} y={NODE_H / 2 - 12} width={24} height={24} className={handled ? 'text-base-400' : ICON_COLOR[state]} />
            <text
              x={44} y={quiet ? NODE_H / 2 + 8 : 27} fontSize={NAME_SIZE} fontWeight={600}
              className={state === 'ok' || kind === 'internet' ? 'fill-base-200' : 'fill-base-50'}
            >
              {fit(name)}
            </text>
            {!quiet && (
              <text
                x={44} y={50} fontSize={15} fontWeight={700}
                className={handled ? 'fill-base-300' : LABEL_FILL[state]}
              >
                {caused ? '상위 장비 영향' : handled ? `${LABEL[state]} · 확인함` : LABEL[state]}
              </text>
            )}
          </g>
        )
      })}
    </svg>
  )
}
