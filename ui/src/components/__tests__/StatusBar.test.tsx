import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { StatusBar } from '../StatusBar'
import type { Status } from '../../types'

const makeStatus = (overrides?: Partial<Status>): Status => ({
  iface: 'eth0',
  subnet: '192.168.1.0/24',
  last_scan: null,
  ...overrides,
})

describe('StatusBar', () => {
  it('shows "connecting…" when status is null', () => {
    render(<StatusBar status={null} />)
    expect(screen.getByText('connecting…')).toBeInTheDocument()
  })

  it('shows interface name and subnet when status is provided', () => {
    render(<StatusBar status={makeStatus()} />)
    expect(screen.getByText('eth0')).toBeInTheDocument()
    expect(screen.getByText('192.168.1.0/24')).toBeInTheDocument()
  })

  it('shows "iface" and "subnet" labels', () => {
    render(<StatusBar status={makeStatus()} />)
    expect(screen.getByText('iface')).toBeInTheDocument()
    expect(screen.getByText('subnet')).toBeInTheDocument()
  })

  it('shows last scan time when last_scan is provided', () => {
    // Use a timestamp that will produce a relative time string
    const recentTime = new Date(Date.now() - 30 * 1000).toISOString() // 30s ago
    render(<StatusBar status={makeStatus({ last_scan: recentTime })} />)
    // The component shows "last scan Xs ago"
    expect(screen.getByText(/last scan/)).toBeInTheDocument()
  })

  it('does NOT show last scan line when last_scan is null', () => {
    render(<StatusBar status={makeStatus({ last_scan: null })} />)
    expect(screen.queryByText(/last scan/)).not.toBeInTheDocument()
  })
})
