import { mount } from '@vue/test-utils'
import RuntimeBadge from './RuntimeBadge.vue'

describe('RuntimeBadge', () => {
  it('shows the published Epoch when the Zone is ready', () => {
    const wrapper = mount(RuntimeBadge, {
      props: { ready: true, epochId: 7 },
    })

    expect(wrapper.text()).toContain('Epoch')
    expect(wrapper.text()).toContain('7')
    expect(wrapper.classes()).toContain('ready')
  })

  it('gives direction when no unified version has been published', () => {
    const wrapper = mount(RuntimeBadge, { props: { ready: false } })
    expect(wrapper.text()).toContain('尚未发布统一版本')
  })
})
