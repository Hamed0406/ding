import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ScanButton } from '../ScanButton'

describe('ScanButton', () => {
  it('renders "Scan now" text when not scanning', () => {
    render(<ScanButton scanning={false} onScan={() => {}} />)
    expect(screen.getByText('Scan now')).toBeInTheDocument()
  })

  it('renders "Scanning…" text when scanning=true', () => {
    render(<ScanButton scanning={true} onScan={() => {}} />)
    expect(screen.getByText('Scanning…')).toBeInTheDocument()
  })

  it('button is disabled when scanning=true', () => {
    render(<ScanButton scanning={true} onScan={() => {}} />)
    expect(screen.getByRole('button')).toBeDisabled()
  })

  it('button is enabled when scanning=false', () => {
    render(<ScanButton scanning={false} onScan={() => {}} />)
    expect(screen.getByRole('button')).not.toBeDisabled()
  })

  it('calls onScan when clicked', async () => {
    const user = userEvent.setup()
    const onScan = vi.fn()
    render(<ScanButton scanning={false} onScan={onScan} />)
    await user.click(screen.getByRole('button'))
    expect(onScan).toHaveBeenCalledTimes(1)
  })

  it('does NOT call onScan when disabled (scanning=true)', async () => {
    const user = userEvent.setup()
    const onScan = vi.fn()
    render(<ScanButton scanning={true} onScan={onScan} />)
    await user.click(screen.getByRole('button'))
    expect(onScan).not.toHaveBeenCalled()
  })
})
