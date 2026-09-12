import { flushPromises, mount } from '@vue/test-utils'
import DocumentsPage from './DocumentsPage.vue'

const apiMock = vi.hoisted(() => ({
  documents: vi.fn(),
  submitDocument: vi.fn(),
}))

vi.mock('../api/client', () => ({
  api: apiMock,
  ApiError: class ApiError extends Error {
    constructor(public failure: { code: string, message: string }) {
      super(failure.message)
    }
  },
}))

beforeEach(() => {
  apiMock.documents.mockReset().mockResolvedValue({
    zone_id: 'zone-1',
    offset: 0,
    has_more: false,
    documents: [{
      document_id: 'document-existing',
      name: 'existing.md',
      media_type: 'text/markdown',
      size: 128,
      content_digest: 'abc123',
      selected: true,
    }],
  })
  apiMock.submitDocument.mockReset().mockResolvedValue({
    zone_id: 'zone-1',
    document_id: 'document-new',
    content_digest: 'def456',
    status: 'uploaded',
  })
})

it('lists the current Zone document membership', async () => {
  const wrapper = mount(DocumentsPage)
  await flushPromises()

  expect(wrapper.find('.document-list').text()).toContain('existing.md')
  expect(wrapper.find('.selection-state').text()).toContain('已在 Zone')
})

it('uploads one document directly into the current Zone', async () => {
  const wrapper = mount(DocumentsPage)
  await flushPromises()
  const file = new File(['hello'], 'notes.md', { type: 'text/markdown' })
  const input = wrapper.find<HTMLInputElement>('input[type="file"]')
  Object.defineProperty(input.element, 'files', { value: [file], configurable: true })
  await input.trigger('change')
  await wrapper.find('form').trigger('submit')
  await flushPromises()

  expect(apiMock.submitDocument).toHaveBeenCalledWith(file)
  expect(wrapper.find('.receipt-card').text()).toContain('文件已加入当前 Zone')
})
