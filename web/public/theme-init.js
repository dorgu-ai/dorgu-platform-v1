// Applies the saved theme before first paint, so a dark-mode user never sees a
// white flash. It is a separate file because the dashboard's content security
// policy allows no inline script. Keep it in step with src/lib/theme.ts.
;(function () {
  var stored = null
  try {
    stored = localStorage.getItem('dorgu-theme')
  } catch (_) {
    stored = null
  }
  var dark =
    stored === 'dark' ||
    ((stored === null || stored === 'system') &&
      window.matchMedia('(prefers-color-scheme: dark)').matches)
  document.documentElement.classList.toggle('dark', dark)
})()
