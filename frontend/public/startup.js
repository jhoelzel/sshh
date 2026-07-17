(() => {
  const showStartupFailure = (reason) => {
    const root = document.getElementById('root')
    if (!root || root.childElementCount > 0) return
    const message = reason instanceof Error ? reason.message : String(reason || 'Frontend modules did not load.')
    root.innerHTML = '<main class="startup-failure" role="alert"><strong>shh-h could not start</strong><span></span></main>'
    root.querySelector('span').textContent = message
  }

  window.addEventListener('error', (event) => showStartupFailure(event.error || event.message))
  window.addEventListener('unhandledrejection', (event) => showStartupFailure(event.reason))
  window.setTimeout(() => showStartupFailure('Frontend initialization timed out.'), 5000)
})()
