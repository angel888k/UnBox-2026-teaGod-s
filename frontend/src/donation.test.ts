import { describe, expect, it } from 'vitest'
import { donationView, formatUpdatedAt, normalizeLeaderboard, type DonationDonor } from './donation'

const 热心网友: DonationDonor = { ID: '', Name: '热心网友', Avatar: '', Anonymous: true }

describe('normalizeLeaderboard', () => {
  it('空值或缺少字段时返回空榜', () => {
    expect(normalizeLeaderboard(null)).toEqual({ UpdatedAt: '', Donors: [] })
    expect(normalizeLeaderboard({})).toEqual({ UpdatedAt: '', Donors: [] })
    expect(normalizeLeaderboard({ Donors: null })).toEqual({ UpdatedAt: '', Donors: [] })
    expect(normalizeLeaderboard({ Donors: [] })).toEqual({ UpdatedAt: '', Donors: [] })
  })

  it('保留后端返回的金额排序', () => {
    const leaderboard = normalizeLeaderboard({
      UpdatedAt: '2026-09-20T12:00:00+08:00',
      Donors: [
        { ID: 'z', Name: 'Zed', Avatar: 'z.png', Anonymous: false },
        { ID: 'c', Name: 'Cara', Avatar: 'c.png', Anonymous: false },
        { ID: 'a', Name: 'Amy', Avatar: 'a.png', Anonymous: false },
      ],
    })

    expect(leaderboard.UpdatedAt).toBe('2026-09-20T12:00:00+08:00')
    expect(leaderboard.Donors).toEqual([
      { ID: 'z', Name: 'Zed', Avatar: 'z.png', Anonymous: false },
      { ID: 'c', Name: 'Cara', Avatar: 'c.png', Anonymous: false },
      { ID: 'a', Name: 'Amy', Avatar: 'a.png', Anonymous: false },
    ])
    expect(leaderboard.Donors.every((donor) => !Object.prototype.hasOwnProperty.call(donor, 'Amount'))).toBe(true)
  })

  it('非法更新时间归一为未知', () => {
    const leaderboard = normalizeLeaderboard({ UpdatedAt: 'not-a-date', Donors: [] })

    expect(leaderboard.UpdatedAt).toBe('未知')
  })

  it('字段归一化时保持输入顺序', () => {
    const leaderboard = normalizeLeaderboard({
      Donors: [
        { ID: 'first', Name: '同名', Avatar: 'first.png', Anonymous: false },
        { ID: 'second', Name: '同名', Avatar: 'second.png', Anonymous: false },
        { ID: 'third', Name: '同名', Avatar: 'third.png', Anonymous: false },
      ],
    })

    expect(leaderboard.Donors.map((donor) => donor.ID)).toEqual(['first', 'second', 'third'])
  })

  it('匿名条目不暴露 ID、头像和真实昵称', () => {
    const leaderboard = normalizeLeaderboard({
      Donors: [
        {
          ID: 'secret-id',
          Name: '真实姓名',
          Avatar: 'https://example.com/avatar.png',
          Anonymous: true,
        },
      ],
    })

    expect(leaderboard.Donors).toEqual([
      { ID: '', Name: '热心网友', Avatar: '', Anonymous: true },
    ])
  })

  it('缺失或全空白昵称时回退为热心网友', () => {
    const leaderboard = normalizeLeaderboard({
      Donors: [
        {},
        { ID: 'blank-name', Name: '   ', Avatar: 'blank.png', Anonymous: false },
      ],
    })

    expect(leaderboard.Donors).toEqual([
      { ID: '', Name: '热心网友', Avatar: '', Anonymous: false },
      { ID: 'blank-name', Name: '热心网友', Avatar: 'blank.png', Anonymous: false },
    ])
  })
})

describe('formatUpdatedAt', () => {
  it('空值或非法值返回未知', () => {
    expect(formatUpdatedAt('')).toBe('未知')
    expect(formatUpdatedAt(null)).toBe('未知')
    expect(formatUpdatedAt('not-a-date')).toBe('未知')
  })

  it('正常 RFC3339 时间返回可读文本', () => {
    const formatted = formatUpdatedAt('2026-09-20T12:34:56+08:00')

    expect(formatted).toMatch(/^2026-09-20 \d{2}:\d{2}$/)
  })
})

describe('donationView', () => {
  // 拉取期间 Donors 还是空数组，若直接按长度判断会把「加载中」显示成「还没人捐助」。
  it('加载中且尚无数据时显示加载中', () => {
    expect(donationView('loading', [])).toBe('loading')
  })

  it('加载中但已有上次成功的数据时继续展示列表', () => {
    expect(donationView('loading', [热心网友])).toBe('list')
  })

  // 失败保留旧数据：宁可展示略旧的榜单，也不回退成空态或错误文案。
  it('加载失败但已有旧数据时继续展示列表', () => {
    expect(donationView('failed', [热心网友])).toBe('list')
  })

  it('加载失败且无数据时显示错误', () => {
    expect(donationView('failed', [])).toBe('error')
  })

  it('成功但榜单为空时显示空态', () => {
    expect(donationView('ready', [])).toBe('empty')
  })

  it('尚未拉取过时不显示错误', () => {
    expect(donationView('idle', [])).toBe('empty')
  })
})
