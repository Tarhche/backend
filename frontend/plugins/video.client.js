import VueVideoPlayer from '@videojs-player/vue'
import 'video.js/dist/video-js.css'

export default defineNuxtPlugin(nuxtApp => {
     nuxtApp.vueApp.use(VueVideoPlayer, {
        //     controls: true,
        //     preload: 'auto',
        //     liveui: true,
        //     nativeControlsForTouch: true,
        //     playbackRates: [0.5, 1, 1.25, 1.5, 2],
        //     playsinline: true,
        //     preferFullWindow: true,
        //     responsive: true,
        //     controlBar: {
        //         skipButtons: {
        //             forward: 10,
        //             backward: 30
        //         }
        //     }
        }
    )
})