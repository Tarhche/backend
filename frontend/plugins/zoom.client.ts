import {defineNuxtPlugin} from '#app'
import mediumZoom, {Zoom} from 'medium-zoom'

export default defineNuxtPlugin((nuxtApp) => {
    const selector = '.image-zoomable, .image img'
    const zoom: Zoom = mediumZoom(selector, {})

    // (re-)init for newly rendered page, also to work in SPA mode (client-side routing)
    nuxtApp.hook('page:finish', () => {
        zoom.detach(selector).attach(selector)
    })

    // make available as helper to NuxtApp
    nuxtApp.provide('mediumZoom', zoom)
})

// more details:
// https://thriving.dev/blog/nuxt3-plugin-medium-zoom
// https://github.com/francoischalifour/medium-zoom
