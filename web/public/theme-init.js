try {
  document.documentElement.dataset.theme = localStorage.getItem('owlmon-theme') ||
    (matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark')
} catch { document.documentElement.dataset.theme = 'dark' }
