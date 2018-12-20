/* Relative Error Rounder
 *   It convert a number to string and always keep rounding
 *   error less tham some predefined value (default 0.1)
 *   For number >=1000 it just applies AngularJS number filter
 *   TODO should not require AngularJS Number filter
 * Functions:
 * - get(options) - returns a rounder functions
 *   options:
 *   - rre - Relative Rounding Error (default 0.1)
 *   - trailingZeros - Show trailing zeros (default false)
 *
 * Examples:
 * let r = Rounder.get()
 * r(0.01234) // -> '0.012'
 * ----
 * let r = Rounder.get({rre: 0.01, showTrailingZeros: true})
 * r(0.01) // -> '0.0100'
*/
mciModule.factory('Rounder', function(numberFilter) {
  const defaults = {
    rre: 0.1,
    trailingZeros: false,
  }

  const expBreakPoint = 2

  function get(options) {
    options = Object.assign({}, defaults, options || {})
    let takeN = Math.abs(Math.log10(options.rre)) + 1

    return function(v) {
      let absV = Math.abs(v)
      let exponent = Math.floor(Math.log10(absV))
      // FIXME may produce less precise results for values > 1000
      //       and rre < 0.0001
      if (exponent > expBreakPoint) { return numberFilter(v, 0) }

      // Perorm conversion to string and rounding
      let mantissaS = Math.round(absV / Math.pow(10, exponent - takeN + 1))
        .toString()
        .substr(0, takeN)

      // trailingZeros option
      if (!options.trailingZeros) {
        mantissaS = mantissaS.replace(/0+$/, '')
      }

      let sign = Math.sign(v) == -1 ? '-' : ''

      // Output formatting
      return exponent < 0
        ? sign + '0.' + '0'.repeat(-exponent - 1) + mantissaS
        : sign + mantissaS.substr(0, exponent + 1) + (
          mantissaS.length > exponent + 1
            ? '.' + mantissaS.substr(exponent + 1) 
            : '0'.repeat(Math.max(0, exponent - mantissaS.length + 1))
        )
    }
  }

  return {get: get}
})
