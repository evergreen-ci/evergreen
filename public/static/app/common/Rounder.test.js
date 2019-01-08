describe('RounderTest', function() {
  beforeEach(module('MCI'));

  let Rounder

  beforeEach(inject(function($injector) {
    Rounder = $injector.get('Rounder')
  }))

  it('has default options', function() {
    expect(Rounder.get()(0.1)).toBe('0.1')
  })

  it('respects rre option', function() {
    let r = Rounder.get({rre: 0.0001})
    expect(r(0.123456789)).toBe('0.12346')
  })

  it('respects trailingZeros option', function() {
    let r = Rounder.get({trailingZeros: true})
    expect(r(0.1)).toBe('0.10')
  })

  it('pretty prints lon numbers', function() {
    let r = Rounder.get()
    expect(r(1000)).toBe('1,000')
    expect(r(12345)).toBe('12,345')
    expect(r(1234567)).toBe('1,234,567')
  })

  it('rounds value to closest one in terms of required precision', function() {
    let r = Rounder.get()
    expect(r(0.100)).toBe('0.1')
    expect(r(0.104)).toBe('0.1')
    expect(r(0.104)).toBe('0.1')
    expect(r(0.105)).toBe('0.11')
    expect(r(0.109)).toBe('0.11')
  })

  it('keeps rounding error consistent', function() {
    let r = Rounder.get({rre: 0.01})
    expect(r(0.1)).toBe('0.1')
    expect(r(0.12)).toBe('0.12')
    expect(r(0.123)).toBe('0.123')
    expect(r(0.1234)).toBe('0.123')

    expect(r(0.01234)).toBe('0.0123')
    expect(r(0.001234)).toBe('0.00123')
    expect(r(0.0001234)).toBe('0.000123')
  })

  it('correctly round numbers which contain both integer and fract part', function() {
    let r = Rounder.get({rre: 0.01})
    expect(r(1)).toBe('1')
    expect(r(1.1)).toBe('1.1')
    expect(r(1.012)).toBe('1.01')
    expect(r(1.0012)).toBe('1')
    expect(r(10.12)).toBe('10.1')
    expect(r(10)).toBe('10')
    expect(r(100.12)).toBe('100')
  })

  it('accepts negative values', function() {
    let r = Rounder.get()
    expect(r(-1.5)).toBe('-1.5')
  })

  it('uses "round half away from zero" rounding strategy', function() {
    let r = Rounder.get()
    expect(r(0.105)).toBe('0.11')
    expect(r(-0.105)).toBe('-0.11')
  })
})
