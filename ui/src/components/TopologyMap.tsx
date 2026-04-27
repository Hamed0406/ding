// ============================================================
// ui/src/components/TopologyMap.tsx — Network topology visualization
//
// Renders an interactive SVG star-topology map:
//   - Gateway node at the center (large, cyan)
//   - Device nodes arranged in a circle around it
//   - Edges drawn as lines from gateway to each device
//   - Hover tooltips with device details
//   - Color-coded: alive=green, unreachable=grey, gateway=cyan
//
// No external graph library needed — pure SVG + React.
// ============================================================

import { useEffect, useState } from 'react'
import { fetchTopology } from '../api/client'
import type { TopologyGraph, TopologyNode } from '../types'
import { portName } from '../utils/ports'

// Layout constants
const WIDTH = 900
const HEIGHT = 600
const CENTER_X = WIDTH / 2
const CENTER_Y = HEIGHT / 2
const RADIUS = Math.min(WIDTH, HEIGHT) * 0.35 // orbit radius for device nodes
const GATEWAY_R = 30  // gateway node radius
const DEVICE_R = 18   // device node radius

interface PositionedNode extends TopologyNode {
  x: number
  y: number
  r: number
}

interface Props {
  scanCount: number // increments after each scan; triggers a re-fetch
}

export function TopologyMap({ scanCount }: Props) {
  const [graph, setGraph] = useState<TopologyGraph | null>(null)
  const [hoveredNode, setHoveredNode] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    fetchTopology()
      .then(setGraph)
      .catch((e) => setError(e.message))
  }, [scanCount])

  if (error) {
    return (
      <div className="text-center py-12 text-red-400">
        Failed to load topology: {error}
      </div>
    )
  }

  if (!graph || graph.nodes.length === 0) {
    return (
      <div className="text-center py-12 text-slate-500">
        No topology data yet — run a scan first
      </div>
    )
  }

  // Position nodes: gateway at center, devices in a circle
  const positioned = layoutNodes(graph)
  const hovered = positioned.find((n) => n.id === hoveredNode)

  return (
    <div className="w-full">
      <svg
        viewBox={`0 0 ${WIDTH} ${HEIGHT}`}
        className="w-full rounded-xl bg-slate-800/50 border border-slate-700"
        style={{ maxHeight: '70vh' }}
      >
        {/* Edges */}
        {graph.edges.map((edge) => {
          const from = positioned.find((n) => n.id === edge.from)
          const to = positioned.find((n) => n.id === edge.to)
          if (!from || !to) return null
          const isHighlighted = hoveredNode === edge.from || hoveredNode === edge.to
          return (
            <line
              key={`${edge.from}-${edge.to}`}
              x1={from.x}
              y1={from.y}
              x2={to.x}
              y2={to.y}
              stroke={isHighlighted ? '#22d3ee' : '#334155'}
              strokeWidth={isHighlighted ? 2 : 1}
              strokeDasharray={isHighlighted ? undefined : '4 4'}
              className="transition-all duration-200"
            />
          )
        })}

        {/* Nodes */}
        {positioned.map((node) => (
          <g
            key={node.id}
            onMouseEnter={() => setHoveredNode(node.id)}
            onMouseLeave={() => setHoveredNode(null)}
            className="cursor-pointer"
          >
            {/* Glow ring on hover */}
            {hoveredNode === node.id && (
              <circle
                cx={node.x}
                cy={node.y}
                r={node.r + 6}
                fill="none"
                stroke={nodeColor(node)}
                strokeWidth={2}
                opacity={0.4}
              />
            )}

            {/* Node circle */}
            <circle
              cx={node.x}
              cy={node.y}
              r={node.r}
              fill={nodeFill(node)}
              stroke={nodeColor(node)}
              strokeWidth={2}
            />

            {/* Icon: 🌐 for gateway, ● for devices */}
            <text
              x={node.x}
              y={node.y}
              textAnchor="middle"
              dominantBaseline="central"
              fontSize={node.type === 'gateway' ? 20 : 12}
              className="select-none pointer-events-none"
            >
              {node.type === 'gateway' ? '🌐' : (node.alive ? '●' : '○')}
            </text>

            {/* Label below node */}
            <text
              x={node.x}
              y={node.y + node.r + 14}
              textAnchor="middle"
              fontSize={11}
              fill="#94a3b8"
              className="pointer-events-none"
            >
              {node.label.length > 16 ? node.label.slice(0, 14) + '…' : node.label}
            </text>

            {/* Port pills below label for gateway */}
            {node.type === 'gateway' && node.open_ports && node.open_ports.length > 0 && (
              <text
                x={node.x}
                y={node.y + node.r + 28}
                textAnchor="middle"
                fontSize={9}
                fill="#67e8f9"
                className="pointer-events-none"
              >
                {node.open_ports.slice(0, 5).map(portName).join(' · ')}
              </text>
            )}
          </g>
        ))}

        {/* Tooltip panel */}
        {hovered && <TooltipPanel node={hovered} />}

        {/* Legend */}
        <g transform={`translate(16, ${HEIGHT - 70})`}>
          <rect x={0} y={0} width={160} height={60} rx={8} fill="#0f172a" opacity={0.8} />
          <circle cx={16} cy={16} r={6} fill="#164e63" stroke="#22d3ee" strokeWidth={1.5} />
          <text x={28} y={20} fontSize={10} fill="#94a3b8">Gateway</text>
          <circle cx={16} cy={34} r={5} fill="#052e16" stroke="#22c55e" strokeWidth={1.5} />
          <text x={28} y={38} fontSize={10} fill="#94a3b8">Device (alive)</text>
          <circle cx={100} cy={34} r={5} fill="#1e293b" stroke="#475569" strokeWidth={1.5} />
          <text x={112} y={38} fontSize={10} fill="#94a3b8">Offline</text>
        </g>
      </svg>
    </div>
  )
}

// --- Tooltip panel shown on hover ---
function TooltipPanel({ node }: { node: PositionedNode }) {
  const tx = Math.min(node.x + node.r + 12, WIDTH - 200)
  const ty = Math.max(node.y - 40, 10)

  return (
    <g transform={`translate(${tx}, ${ty})`}>
      <rect
        x={0} y={0}
        width={185} height={node.open_ports && node.open_ports.length > 0 ? 100 : 76}
        rx={8}
        fill="#0f172a"
        stroke="#334155"
        strokeWidth={1}
        opacity={0.95}
      />
      <text x={10} y={20} fontSize={12} fontWeight="bold" fill="#e2e8f0">
        {node.label}
      </text>
      <text x={10} y={36} fontSize={10} fill="#94a3b8" fontFamily="monospace">
        IP: {node.id}
      </text>
      <text x={10} y={50} fontSize={10} fill="#94a3b8" fontFamily="monospace">
        MAC: {node.mac || '—'}
      </text>
      <text x={10} y={64} fontSize={10} fill="#94a3b8">
        {node.vendor || 'Unknown vendor'} · {node.alive ? '✓ alive' : '✗ offline'}
      </text>
      {node.open_ports && node.open_ports.length > 0 && (
        <text x={10} y={88} fontSize={10} fill="#67e8f9" fontFamily="monospace">
          Ports: {node.open_ports.slice(0, 6).map(portName).join(', ')}
        </text>
      )}
    </g>
  )
}

// --- Layout helpers ---

function layoutNodes(graph: TopologyGraph): PositionedNode[] {
  const gateway = graph.nodes.find((n) => n.type === 'gateway')
  const devices = graph.nodes.filter((n) => n.type === 'device')

  const result: PositionedNode[] = []

  // Gateway at center
  if (gateway) {
    result.push({ ...gateway, x: CENTER_X, y: CENTER_Y, r: GATEWAY_R })
  }

  // Devices in a circle around the gateway
  const count = devices.length
  devices.forEach((device, i) => {
    const angle = (2 * Math.PI * i) / count - Math.PI / 2 // start from top
    result.push({
      ...device,
      x: CENTER_X + RADIUS * Math.cos(angle),
      y: CENTER_Y + RADIUS * Math.sin(angle),
      r: DEVICE_R,
    })
  })

  return result
}

function nodeColor(node: TopologyNode): string {
  if (node.type === 'gateway') return '#22d3ee' // cyan
  return node.alive ? '#22c55e' : '#475569'     // green or grey
}

function nodeFill(node: TopologyNode): string {
  if (node.type === 'gateway') return '#164e63' // dark cyan
  return node.alive ? '#052e16' : '#1e293b'     // dark green or dark slate
}
