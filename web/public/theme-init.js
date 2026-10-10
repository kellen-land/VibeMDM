// Apply the saved theme before first paint (dark-first default) to avoid a flash.
// Kept as a static file so the Content-Security-Policy can forbid inline scripts.
(function () {
  try {
    var t = localStorage.getItem('vibemdm-theme')
    if (t !== 'light' && t !== 'dark') t = 'dark'
    document.documentElement.dataset.theme = t
    document.documentElement.style.colorScheme = t
  } catch (e) {
    document.documentElement.dataset.theme = 'dark'
  }
})()
