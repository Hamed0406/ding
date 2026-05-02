import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ChangesFeed } from '../ChangesFeed'
import type { Change } from '../../types'

describe('ChangesFeed', () => {
  it('renders nothing when changes array is empty', () => {
    const { container } = render(<ChangesFeed changes={[]} />)
    expect(container.firstChild).toBeNull()
  })

  it('renders "Changes" heading when there are changes', () => {
    const changes: Change[] = [{ kind: 'NEW', ip: '192.168.1.5', desc: 'joined' }]
    render(<ChangesFeed changes={changes} />)
    expect(screen.getByText('Changes')).toBeInTheDocument()
  })

  it('renders NEW change with green badge and correct IP/desc', () => {
    const changes: Change[] = [{ kind: 'NEW', ip: '192.168.1.5', desc: 'new device joined' }]
    render(<ChangesFeed changes={changes} />)
    expect(screen.getByText('NEW')).toBeInTheDocument()
    expect(screen.getByText('192.168.1.5')).toBeInTheDocument()
    expect(screen.getByText('new device joined')).toBeInTheDocument()
  })

  it('renders GONE change with correct badge and IP', () => {
    const changes: Change[] = [{ kind: 'GONE', ip: '192.168.1.10', desc: 'device left' }]
    render(<ChangesFeed changes={changes} />)
    expect(screen.getByText('GONE')).toBeInTheDocument()
    expect(screen.getByText('192.168.1.10')).toBeInTheDocument()
    expect(screen.getByText('device left')).toBeInTheDocument()
  })

  it('renders BACK change with correct badge and IP', () => {
    const changes: Change[] = [{ kind: 'BACK', ip: '192.168.1.20', desc: 'device returned' }]
    render(<ChangesFeed changes={changes} />)
    expect(screen.getByText('BACK')).toBeInTheDocument()
    expect(screen.getByText('192.168.1.20')).toBeInTheDocument()
  })

  it('renders PORTS change with correct badge', () => {
    const changes: Change[] = [{ kind: 'PORTS', ip: '192.168.1.30', desc: 'ports=[22,80]' }]
    render(<ChangesFeed changes={changes} />)
    expect(screen.getByText('PORTS')).toBeInTheDocument()
    expect(screen.getByText('192.168.1.30')).toBeInTheDocument()
    expect(screen.getByText('ports=[22,80]')).toBeInTheDocument()
  })

  it('renders MAC_CHANGE change with correct badge', () => {
    const changes: Change[] = [{ kind: 'MAC_CHANGE', ip: '192.168.1.40', desc: 'mac changed' }]
    render(<ChangesFeed changes={changes} />)
    expect(screen.getByText('MAC_CHANGE')).toBeInTheDocument()
    expect(screen.getByText('192.168.1.40')).toBeInTheDocument()
  })

  it('renders multiple changes', () => {
    const changes: Change[] = [
      { kind: 'NEW', ip: '192.168.1.1', desc: 'joined' },
      { kind: 'GONE', ip: '192.168.1.2', desc: 'left' },
      { kind: 'PORTS', ip: '192.168.1.3', desc: 'ports changed' },
    ]
    render(<ChangesFeed changes={changes} />)
    expect(screen.getByText('NEW')).toBeInTheDocument()
    expect(screen.getByText('GONE')).toBeInTheDocument()
    expect(screen.getByText('PORTS')).toBeInTheDocument()
    expect(screen.getByText('192.168.1.1')).toBeInTheDocument()
    expect(screen.getByText('192.168.1.2')).toBeInTheDocument()
    expect(screen.getByText('192.168.1.3')).toBeInTheDocument()
  })
})
