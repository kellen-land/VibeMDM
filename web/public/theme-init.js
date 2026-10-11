// Apply the theme before first paint to avoid a flash: the saved choice, or the operating
// system's appearance when the choice is "system" (the default). ThemeContext keeps it live.
// Kept as a static file so the Content-Security-Policy can forbid inline scripts.
(function () {
  var t = 'dark'
  try {
    var p = localStorage.getItem('vibemdm-theme-preference')
    if (p === 'light' || p === 'dark') t = p
    else if (window.matchMedia && !window.matchMedia('(prefers-color-scheme: dark)').matches) t = 'light'
  } catch (e) {
    if (window.matchMedia && !window.matchMedia('(prefers-color-scheme: dark)').matches) t = 'light'
  }
  document.documentElement.dataset.theme = t
  document.documentElement.style.colorScheme = t
})()
