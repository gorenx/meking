import { flushPromises, mount } from '@vue/test-utils'
import ControlPage from './ControlPage.vue'
import type { ControlAction, ControlActionStatus, ControlPolicyMode } from '../api/types'

const apiMock = vi.hoisted(() => ({
  controlActions: vi.fn(),
  invokeControlAction: vi.fn(),
  publishControlPolicy: vi.fn(),
}))

vi.mock('../api/client', () => ({
  api: apiMock,
  ApiError: class ApiError extends Error {
    constructor(public failure: { code: string, message: string }) {
      super(failure.message)
    }
  },
}))

const actions: ControlAction[] = [
  'convert_document',
  'create_text_units',
  'extract_knowledge',
  'index_entity_vectors',
  'derive_community_structure',
  'publish_epoch',
  'generate_community_reports',
]

function status(
  action: ControlAction,
  mode: ControlPolicyMode = 'automatic',
  pendingCount = 0,
): ControlActionStatus {
  return {
    zone_id: 'zone-1',
    action,
    pending_count: pendingCount,
    pending_since: pendingCount ? '2026-09-01T01:02:03Z' : undefined,
    policy: {
      action,
      mode,
      minimum_pending: mode === 'automatic' ? 3 : 0,
      maximum_wait: mode === 'automatic' ? '5m0s' : '0s',
      revision: 2,
      updated_at: '2026-09-01T01:00:00Z',
    },
  }
}

beforeEach(() => {
  apiMock.controlActions.mockReset().mockResolvedValue({
    actions: actions.map((action) => status(action, 'automatic', action === 'extract_knowledge' ? 3 : 0)),
  })
  apiMock.invokeControlAction.mockReset().mockResolvedValue({
    action: 'extract_knowledge',
  })
  apiMock.publishControlPolicy.mockReset()
})

it('renders all seven Actions before any Policy is configured', async () => {
  apiMock.controlActions.mockResolvedValue({ actions: [] })
  const wrapper = mount(ControlPage)
  await flushPromises()

  expect(wrapper.findAll('.control-action')).toHaveLength(7)
  expect(wrapper.findAll('.policy-mode.mode-unconfigured')).toHaveLength(7)
  wrapper.unmount()
})

it('invokes a Manual Action with pending input', async () => {
  apiMock.controlActions.mockResolvedValue({
    actions: actions.map((action) => status(
      action,
      action === 'extract_knowledge' ? 'manual' : 'automatic',
      action === 'extract_knowledge' ? 3 : 0,
    )),
  })
  const wrapper = mount(ControlPage)
  await flushPromises()
  const extract = wrapper.findAll('.control-action').find((item) => item.text().includes('抽取知识'))

  await extract!.find('.submit-button').trigger('click')
  await flushPromises()

  expect(apiMock.invokeControlAction).toHaveBeenCalledWith('extract_knowledge')
  expect(wrapper.find('.control-receipt').text()).toContain('当前 Zone')
  expect(extract!.find('.control-action-receipt').text()).toContain('已受理')
  expect(extract!.find('.control-action-receipt').text()).toContain('处理成功前')
  expect(extract!.find('.submit-button').text()).toBe('再次触发')
  wrapper.unmount()
})

it('publishes the selected Project Policy revision', async () => {
  apiMock.publishControlPolicy.mockResolvedValue({
    ...status('convert_document', 'manual').policy,
    revision: 3,
  })
  const wrapper = mount(ControlPage)
  await flushPromises()

  await wrapper.find('.control-action .text-button').trigger('click')
  await wrapper.findAll('.policy-mode-picker button')[1].trigger('click')
  await wrapper.find('.policy-editor form').trigger('submit')
  await flushPromises()

  expect(apiMock.publishControlPolicy).toHaveBeenCalledWith('convert_document', {
    mode: 'manual',
    minimum_pending: 0,
    maximum_wait: '0s',
    expected_revision: 2,
  })
  wrapper.unmount()
})
