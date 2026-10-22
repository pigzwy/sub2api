// Runs before the page icon and title are painted. The static shell cannot
// embed the live site name, so a refresh reuses the brand saved last visit.
(function () {
  var storageKey = 'sub2api_site_brand'
  try {
    var raw = localStorage.getItem(storageKey)
    if (!raw) return
    var brand = JSON.parse(raw)
    if (!brand || typeof brand !== 'object') return
    if (typeof brand.name === 'string' && brand.name && !document.title) {
      document.title = brand.name
    }
    if (typeof brand.logo === 'string' && brand.logo && !document.querySelector('link[rel="icon"]')) {
      var link = document.createElement('link')
      link.rel = 'icon'
      link.href = brand.logo
      document.head.appendChild(link)
    }
  } catch (e) {
    // Ignore a broken cache and let the app apply the live settings.
  }
})()
