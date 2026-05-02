import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { DeviceCard } from '../DeviceCard'
import type { Device } from '../../types'

vi.mock('../../api/client', () => ({
  scanDevice: vi.fn().mockResolvedValue({ ip: '192.168.1.1', open_ports: [80] }),
  wakeDevice: vi.fn().mockResolvedValue(undefined),
}))

function makeDevice(overrides?: Partial<Device>): Device {
  return {
    ip: '192.168.1.1',
    mac: 'aa:bb:cc:dd:ee:ff',
    hostname: null,
    vendor: null,
    device_type: null,
    os: null,
    label: null,
    notify: true,
    open_ports: [],
    alive: true,
    ...overrides,
  }
}

const noop = () => {}

describe('DeviceCard', () => {
  it('renders IP address', () => {
    render(
      <DeviceCard
        device={makeDevice({ ip: '10.0.0.5' })}
        onLabelChange={noop}
        onNotifyChange={noop}
        onSelect={noop}
      />
    )
    expect(screen.getByText('10.0.0.5')).toBeInTheDocument()
  })

  it('renders hostname when provided', () => {
    render(
      <DeviceCard
        device={makeDevice({ hostname: 'my-laptop.local' })}
        onLabelChange={noop}
        onNotifyChange={noop}
        onSelect={noop}
      />
    )
    expect(screen.getByText('my-laptop.local')).toBeInTheDocument()
  })

  it('renders vendor when provided', () => {
    render(
      <DeviceCard
        device={makeDevice({ vendor: 'Apple, Inc.' })}
        onLabelChange={noop}
        onNotifyChange={noop}
        onSelect={noop}
      />
    )
    expect(screen.getByText(/Apple, Inc\./)).toBeInTheDocument()
  })

  it('renders device type badge when device_type is provided', () => {
    render(
      <DeviceCard
        device={makeDevice({ device_type: 'Router' })}
        onLabelChange={noop}
        onNotifyChange={noop}
        onSelect={noop}
      />
    )
    expect(screen.getByText('Router')).toBeInTheDocument()
  })

  it('renders open ports', () => {
    render(
      <DeviceCard
        device={makeDevice({ open_ports: [22, 80, 443] })}
        onLabelChange={noop}
        onNotifyChange={noop}
        onSelect={noop}
      />
    )
    // portLabel converts port numbers to service names (SSH/22, HTTP/80, HTTPS/443)
    // The port spans have title={String(p)} so we can query by title
    expect(screen.getByTitle('22')).toBeInTheDocument()
    expect(screen.getByTitle('80')).toBeInTheDocument()
    expect(screen.getByTitle('443')).toBeInTheDocument()
  })

  it('shows "online" indicator when alive=true', () => {
    render(
      <DeviceCard
        device={makeDevice({ alive: true })}
        onLabelChange={noop}
        onNotifyChange={noop}
        onSelect={noop}
      />
    )
    // The alive dot has title="alive"
    expect(screen.getByTitle('alive')).toBeInTheDocument()
  })

  it('shows "offline" indicator when alive=false', () => {
    render(
      <DeviceCard
        device={makeDevice({ alive: false })}
        onLabelChange={noop}
        onNotifyChange={noop}
        onSelect={noop}
      />
    )
    expect(screen.getByTitle('unreachable')).toBeInTheDocument()
  })

  it('calls onSelect when card is clicked', async () => {
    const user = userEvent.setup()
    const onSelect = vi.fn()
    render(
      <DeviceCard
        device={makeDevice()}
        onLabelChange={noop}
        onNotifyChange={noop}
        onSelect={onSelect}
      />
    )
    // Click on the IP text (part of the card body, not a button)
    await user.click(screen.getByText('192.168.1.1'))
    expect(onSelect).toHaveBeenCalledTimes(1)
  })

  it('renders label when device has a label', () => {
    render(
      <DeviceCard
        device={makeDevice({ label: 'Living Room TV' })}
        onLabelChange={noop}
        onNotifyChange={noop}
        onSelect={noop}
      />
    )
    expect(screen.getByText('Living Room TV')).toBeInTheDocument()
  })

  it('NEW badge appears when isNew=true', () => {
    render(
      <DeviceCard
        device={makeDevice()}
        isNew={true}
        onLabelChange={noop}
        onNotifyChange={noop}
        onSelect={noop}
      />
    )
    expect(screen.getByText('new')).toBeInTheDocument()
  })
})
