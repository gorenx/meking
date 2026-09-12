import { useRuntime } from './useRuntime'

describe('useRuntime polling', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    useRuntime().stopPolling()
  })

  afterEach(() => {
    useRuntime().stopPolling()
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('refreshes an unpublished runtime until a published Epoch becomes visible', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(runtimeResponse(false, 0))
      .mockResolvedValueOnce(runtimeResponse(true, 8))
    vi.stubGlobal('fetch', fetchMock)

    const state = useRuntime()
    state.startPolling()
    await vi.runOnlyPendingTimersAsync()

    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(state.runtime.value?.ready).toBe(true)
    expect(state.runtime.value?.epoch_id).toBe(8)
  })

  it('does not create duplicate polling loops', async () => {
    const fetchMock = vi.fn().mockResolvedValue(runtimeResponse(true, 3))
    vi.stubGlobal('fetch', fetchMock)

    const state = useRuntime()
    state.startPolling()
    state.startPolling()
    await vi.runOnlyPendingTimersAsync()

    expect(fetchMock).toHaveBeenCalledTimes(2)
  })
})

function runtimeResponse(ready: boolean, epochID: number) {
  return new Response(JSON.stringify({
    contract_version: 11,
    application_version: 'test',
	zone_id: '10000000-0000-4000-8000-000000000001',
    epoch_id: epochID,
    report_set_id: ready ? 'reports' : '',
    ready,
    capabilities: ['basic', 'local', 'global'],
  }), { status: 200, headers: { 'Content-Type': 'application/json' } })
}
