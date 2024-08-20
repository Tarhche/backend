import VuePlyr from '@skjnldsv/vue-plyr'
import '@skjnldsv/vue-plyr/dist/vue-plyr.css'

export default defineNuxtPlugin((nuxtApp) => {
    nuxtApp.vueApp.component('vue-plyr' ,VuePlyr)
})
