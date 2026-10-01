import { describe, expect, it } from 'vitest'
import { buildVodSourceDeadInfo, vodSourceDeadBackLabel, vodSourceDeadBackTarget } from './vodSourceDead'

describe('vodSourceDead', () => {
  it('builds the dialog model with site, title and raw message', () => {
    const info = buildVodSourceDeadInfo('非凡资源', '长安十二时辰', new Error('Get "http://x": context deadline exceeded'))
    expect(info.siteName).toBe('非凡资源')
    expect(info.vodTitle).toBe('长安十二时辰')
    expect(info.message).toContain('context deadline exceeded')
  })

  it('keeps the raw message for non-Error values', () => {
    expect(buildVodSourceDeadInfo('站', '剧', 'network unreachable').message).toBe('network unreachable')
  })

  it('goes back home only when the detail was opened from home', () => {
    // 首页观看记录路径在详情加载前已切到详情页，失败需回首页。
    expect(vodSourceDeadBackTarget('home')).toBe('home')
    // 收藏/搜索/列表路径在详情加载成功前不离开原页面，留在原地即可。
    expect(vodSourceDeadBackTarget('favorites')).toBe('stay')
    expect(vodSourceDeadBackTarget('search')).toBe('stay')
    expect(vodSourceDeadBackTarget('list')).toBe('stay')
  })

  it('labels the back button accordingly', () => {
    expect(vodSourceDeadBackLabel('home')).toBe('返回首页')
    expect(vodSourceDeadBackLabel('favorites')).toBe('关闭')
  })
})
