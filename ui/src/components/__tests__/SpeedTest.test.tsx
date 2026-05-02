import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SpeedTest } from '../SpeedTest'

const { mockFetchHistory, mockRunSpeedtest } = vi.hoisted(() => ({
  mockFetchHistory: vi.fn().mockResolvedValue([]),
  mockRunSpeedtest: vi.fn().mockResolvedValue({
    download_mbps: 125.4,
    upload_mbps: 35.2,
    ping_ms: 12.5,
    server: 'speed.cloudflare.com',
    tested_at: new Date().toISOString(),
  }),
}))

const mockResult = {
  download_mbps: 125.4,
  upload_mbps: 35.2,
  ping_ms: 12.5,
  server: 'speed.cloudflare.com',
  tested_at: new Date().toISOString(),
}

vi.mock('../../api/client', () => ({
  fetchSpeedtestHistory: mockFetchHistory,
  runSpeedtest: mockRunSpeedtest,
}))

beforeEach(() => {
  mockFetchHistory.mockResolvedValue([])
  mockRunSpeedtest.mockResolvedValue(mockResult)
})

describe('SpeedTest', () => {
  it('renders "Run Speed Test" button initially', async () => {
    render(<SpeedTest />)
    await waitFor(() => {
      expect(screen.getByText('Run Speed Test')).toBeInTheDocument()
    })
  })

  it('button is not disabled initially', async () => {
    render(<SpeedTest />)
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /run speed test/i })).not.toBeDisabled()
    })
  })

  it('shows history table when history has more than 1 result', async () => {
    const historyResults = [
      { ...mockResult, tested_at: new Date(Date.now() - 3600000).toISOString() },
      { ...mockResult, download_mbps: 100.0, tested_at: new Date(Date.now() - 7200000).toISOString() },
    ]
    mockFetchHistory.mockResolvedValue(historyResults)
    render(<SpeedTest />)
    await waitFor(() => {
      expect(screen.getByText('History')).toBeInTheDocument()
    })
  })

  it('after clicking Run and resolving, shows download_mbps value', async () => {
    const user = userEvent.setup()
    render(<SpeedTest />)
    // Wait for the initial render to settle
    await waitFor(() => screen.getByText('Run Speed Test'))
    await user.click(screen.getByRole('button', { name: /run speed test/i }))
    // Wait for result to appear (125.4 Mbps download)
    await waitFor(() => {
      expect(screen.getByText('125.4')).toBeInTheDocument()
    })
  })

  it('shows error message when runSpeedtest rejects', async () => {
    mockRunSpeedtest.mockRejectedValue(new Error('Connection timeout'))
    const user = userEvent.setup()
    render(<SpeedTest />)
    await waitFor(() => screen.getByText('Run Speed Test'))
    await user.click(screen.getByRole('button', { name: /run speed test/i }))
    await waitFor(() => {
      expect(screen.getByText('Connection timeout')).toBeInTheDocument()
    })
  })
})
